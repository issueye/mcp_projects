package canvas

import (
	"sort"
	"time"
)

// 按项目分组排版：一块画布上同时展示多个项目时使用。
//
// 背景：项目制落地后，每个项目各自从 (80,80) 开始自动排版 —— 单项目视图没问题，
// 但「全部」视图里不同项目的卡片会完全重叠。这里提供一次性的分行排布：
// 一个项目一行，行内按创建时间横排，超宽换行；行首留出标签位（200px）。
const (
	arrangeStartX = 220 // 行首留白：给左侧项目标签
	arrangeStartY = 60
	arrangeGapX   = 32
	arrangeRowGap = 110 // 行与行之间（含标签高度）
	arrangeMaxW   = 1900
)

// ArrangeByProject 把全部节点按项目分行排布，返回移动过的节点数。
//
// 只处理普通节点；分组框（KindFrame）是自由绘制的背板，不参与自动排版
// —— 它的位置由创建时的成员边界决定，强行重排反而会错位。
func (s *Store) ArrangeByProject() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	projects := make([]*Project, 0, len(s.projects))
	for _, p := range s.projects {
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool {
		if !projects[i].CreatedAt.Equal(projects[j].CreatedAt) {
			return projects[i].CreatedAt.Before(projects[j].CreatedAt)
		}
		return projects[i].ID < projects[j].ID
	})

	moved := 0
	y := float64(arrangeStartY)
	now := time.Now()
	for _, p := range projects {
		nodes := make([]*Node, 0, 8)
		for _, n := range s.nodes {
			if n.ProjectID == p.ID && n.Kind != KindFrame {
				nodes = append(nodes, n)
			}
		}
		if len(nodes) == 0 {
			continue
		}
		sort.Slice(nodes, func(i, j int) bool {
			if !nodes[i].CreatedAt.Equal(nodes[j].CreatedAt) {
				return nodes[i].CreatedAt.Before(nodes[j].CreatedAt)
			}
			return nodes[i].ID < nodes[j].ID
		})

		x := float64(arrangeStartX)
		rowBottom := y
		for _, n := range nodes {
			w := float64(n.Width)
			if x > arrangeStartX && x+w > arrangeMaxW {
				x = arrangeStartX
				y = rowBottom + arrangeRowGap
				rowBottom = y
			}
			if n.X != x || n.Y != y {
				n.X, n.Y = x, y
				n.UpdatedAt = now
				moved++
			}
			x += w + arrangeGapX
			if bottom := y + float64(n.Height); bottom > rowBottom {
				rowBottom = bottom
			}
		}
		y = rowBottom + arrangeRowGap
	}
	if moved > 0 {
		s.persistLocked()
	}
	return moved, nil
}

// SeparateProjectGroups 让「全部」视图里的项目分组不再互相遮挡。
//
// 与 ArrangeByProject 的区别：这里**不重排**组内卡片，只把整组沿 y 方向错开，
// 于是 Agent/用户摆好的列宽、间距、阅读顺序全部保留 —— 之前的分行排版会把
// 1440 宽的卡片硬折成一列，长画布虽然不重叠，但内容全被拆散、更难读。
// 只在包围盒真的相交时移动，因此幂等（没有重叠就返回 0）。
func (s *Store) SeparateProjectGroups() (int, error) {
	const gap = float64(arrangeRowGap)
	s.mu.Lock()
	defer s.mu.Unlock()

	type group struct {
		id  string
		x0  float64
		y0  float64
		x1  float64
		y1  float64
		set []*Node
	}
	index := map[string]*group{}
	for _, n := range s.nodes {
		g := index[n.ProjectID]
		if g == nil {
			g = &group{id: n.ProjectID, x0: n.X, y0: n.Y, x1: n.X + float64(n.Width), y1: n.Y + float64(n.Height)}
			index[n.ProjectID] = g
		}
		if n.X < g.x0 {
			g.x0 = n.X
		}
		if n.Y < g.y0 {
			g.y0 = n.Y
		}
		if r := n.X + float64(n.Width); r > g.x1 {
			g.x1 = r
		}
		if b := n.Y + float64(n.Height); b > g.y1 {
			g.y1 = b
		}
		g.set = append(g.set, n)
	}
	groups := make([]*group, 0, len(index))
	for _, g := range index {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].y0 != groups[j].y0 {
			return groups[i].y0 < groups[j].y0
		}
		return groups[i].id < groups[j].id
	})

	// 先判重叠：完全没重叠时不动任何坐标（幂等 + 不打扰手工排版）
	overlap := false
	for i := 0; i < len(groups) && !overlap; i++ {
		for j := i + 1; j < len(groups); j++ {
			a, b := groups[i], groups[j]
			if a.x0 < b.x1 && b.x0 < a.x1 && a.y0 < b.y1 && b.y0 < a.y1 {
				overlap = true
				break
			}
		}
	}
	if !overlap {
		return 0, nil
	}

	now := time.Now()
	moved := 0
	cursor := groups[0].y0
	type shift struct {
		n    *Node
		from float64
		to   float64
	}
	var shifts []shift
	for _, g := range groups {
		dy := 0.0
		if g.y0 < cursor {
			dy = cursor - g.y0
		}
		if dy > 0 {
			for _, n := range g.set {
				shifts = append(shifts, shift{n: n, from: n.Y, to: n.Y + dy})
				n.Y += dy
				n.UpdatedAt = now
				moved++
			}
			g.y0 += dy
			g.y1 += dy
		}
		if g.y1+gap > cursor {
			cursor = g.y1 + gap
		}
	}
	if moved == 0 {
		return 0, nil
	}
	// 一次整体可撤销：挪分组是可逆的批量操作，不该把历史栈塞满
	s.pushUndoLocked(undoEntry{
		desc: "分开重叠的项目分组",
		undo: func() {
			for _, sh := range shifts {
				sh.n.Y = sh.from
			}
		},
		redo: func() {
			for _, sh := range shifts {
				sh.n.Y = sh.to
			}
		},
	})
	s.persistLocked()
	return moved, nil
}
