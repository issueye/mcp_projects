package canvas

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// 项目导出 / 导入：把一块画布作用域（项目 + 节点正文 + 连线 + 画布版式）
// 打包成可移动的 JSON 文件，并在另一端还原成新的项目。
//
// 设计取舍：
//   - 单文件 JSON 而不是 ZIP：设计稿正文本来就以字符串存在（iframe srcdoc），
//     JSON 能一次带全元数据与正文，不引入压缩/目录结构依赖；浏览器侧直接
//     Blob 下载、FileReader 读取，整条链路无需额外运行时。
//   - 导入永远新建项目，不改动已有项目：同名时自动加「（导入）」后缀，
//     因此导入是纯增量操作，失败时最多留下一个可删除的新项目。
//   - 节点 ID / 连线 ID 全部重新生成，避免与目标实例已有 ID 冲突；
//     坐标、尺寸、层级、标签、画板与 auto_height 原样保留，视觉布局不漂移。
//   - 格式带 format/version：未来字段变化时能给出可执行的错误提示，
//     而不是把半份数据灌进 Store。

const (
	// ProjectBundleFormat 是单个项目导出文件的格式标识。
	ProjectBundleFormat = "adago-design/project"
	// WorkspaceBundleFormat 是「导出全部项目」的格式标识。
	WorkspaceBundleFormat = "adago-design/workspace"
	// ProjectBundleVersion 是当前 bundle 版本。
	ProjectBundleVersion = 1
	// maxImportNodes 限制单次导入的节点数，防误拖超大文件把内存打满
	// （HTTP 请求体本身另有 8MB 上限）。
	maxImportNodes = 2000
	// maxImportProjects 限制单次导入的项目数。
	maxImportProjects = 200
)

var (
	ErrBundleFormat  = errors.New("unsupported project bundle format")
	ErrBundleVersion = errors.New("unsupported project bundle version")
	ErrBundleInvalid = errors.New("invalid project bundle")
	ErrBundleEmpty   = errors.New("project bundle is empty")
)

// ProjectBundle 是一份项目快照（导出 / 导入共用）。
type ProjectBundle struct {
	Format     string    `json:"format"`
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Project    Project   `json:"project"`
	Nodes      []Node    `json:"nodes"`
	Edges      []Edge    `json:"edges,omitempty"`
}

// WorkspaceBundle 是全部项目的快照（「导出全部项目」）。
type WorkspaceBundle struct {
	Format     string          `json:"format"`
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Projects   []ProjectBundle `json:"projects"`
}

// ExportProject 导出单个项目（含正文）。项目不存在时返回 ErrProjectNotFound。
func (s *Store) ExportProject(id string) (ProjectBundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	if !ok {
		return ProjectBundle{}, ErrProjectNotFound
	}
	return s.exportProjectLocked(p), nil
}

// ExportWorkspace 导出全部项目（含空项目定义与正文）。
func (s *Store) ExportWorkspace() (WorkspaceBundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	projects := make([]*Project, 0, len(s.projects))
	for _, p := range s.projects {
		projects = append(projects, p)
	}
	sortProjectsByCreated(projects)
	out := WorkspaceBundle{
		Format: WorkspaceBundleFormat, Version: ProjectBundleVersion,
		ExportedAt: time.Now(), Projects: make([]ProjectBundle, 0, len(projects)),
	}
	for _, p := range projects {
		out.Projects = append(out.Projects, s.exportProjectLocked(p))
	}
	return out, nil
}

// exportProjectLocked 组装单个项目快照（调用方需持有读锁）。
func (s *Store) exportProjectLocked(p *Project) ProjectBundle {
	b := ProjectBundle{
		Format: ProjectBundleFormat, Version: ProjectBundleVersion,
		ExportedAt: time.Now(), Project: p.Clone(),
	}
	nodes := make([]Node, 0, 16)
	for _, n := range s.nodes {
		if n.ProjectID == p.ID {
			nodes = append(nodes, n.Clone())
		}
	}
	sortNodesByCreated(nodes)
	b.Nodes = nodes
	for _, e := range s.edges {
		a, okA := s.nodes[e.From]
		bb, okB := s.nodes[e.To]
		if okA && okB && a.ProjectID == p.ID && bb.ProjectID == p.ID {
			b.Edges = append(b.Edges, e.Clone())
		}
	}
	sortEdgesByID(b.Edges)
	return b
}

// 顺序稳定：导出文件要能 diff / 快照对比，同一份数据两次导出必须字节一致。
// 时间戳可能同毫秒（Windows 时钟约 15.6ms 粒度），因此一律用 ID 做次级键 ——
// 否则 map 遍历顺序会让导出结果抖动。
func sortProjectsByCreated(list []*Project) {
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return list[i].ID < list[j].ID
	})
}

func sortNodesByCreated(list []Node) {
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return list[i].ID < list[j].ID
	})
}

func sortEdgesByID(list []Edge) {
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
}

// ImportProject 从单个 bundle 还原项目，返回新项目的摘要。
// 同名项目自动加「（导入）」后缀，绝不覆盖已有项目。
func (s *Store) ImportProject(b ProjectBundle) (ProjectSummary, error) {
	out, err := s.ImportProjects([]ProjectBundle{b})
	if err != nil {
		return ProjectSummary{}, err
	}
	return out[0], nil
}

// ImportProjects 批量还原项目；全部校验在写入前完成，
// 因此失败时不会留下「导入了一半」的项目。
func (s *Store) ImportProjects(bundles []ProjectBundle) ([]ProjectSummary, error) {
	if len(bundles) == 0 {
		return nil, ErrBundleEmpty
	}
	if len(bundles) > maxImportProjects {
		return nil, fmt.Errorf("%w: 项目数 %d 超过上限 %d", ErrBundleInvalid, len(bundles), maxImportProjects)
	}
	// 1) 只校验、不改状态：任何一份非法都在这里失败
	type prepared struct {
		b      ProjectBundle
		canvas *ProjectCanvas
	}
	items := make([]prepared, 0, len(bundles))
	for _, b := range bundles {
		if b.Format != "" && b.Format != ProjectBundleFormat {
			return nil, fmt.Errorf("%w: %q", ErrBundleFormat, b.Format)
		}
		if b.Version < 0 || b.Version > ProjectBundleVersion {
			return nil, fmt.Errorf("%w: %d（当前支持 %d）", ErrBundleVersion, b.Version, ProjectBundleVersion)
		}
		if strings.TrimSpace(b.Project.Name) == "" {
			return nil, fmt.Errorf("%w: 项目名为空", ErrBundleInvalid)
		}
		if len(b.Nodes) > maxImportNodes {
			return nil, fmt.Errorf("%w: 节点数 %d 超过上限 %d", ErrBundleInvalid, len(b.Nodes), maxImportNodes)
		}
		for _, n := range b.Nodes {
			if n.Kind != "" && !ValidKind(n.Kind) {
				return nil, fmt.Errorf("%w: 节点 %q 的 kind 非法（%s）", ErrBundleInvalid, n.Title, n.Kind)
			}
		}
		cfg, err := NormalizeCanvas(b.Project.Canvas)
		if err != nil {
			return nil, fmt.Errorf("%w: 画布版式非法: %v", ErrBundleInvalid, err)
		}
		items = append(items, prepared{b: b, canvas: cfg})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	before := make(map[string]bool, len(s.projects))
	for id := range s.projects {
		before[id] = true
	}
	out := make([]ProjectSummary, 0, len(items))
	for _, it := range items {
		out = append(out, s.importProjectLocked(it.b, it.canvas, now))
	}
	s.pushImportUndoLocked(before)
	s.persistLocked()
	return out, nil
}

// pushImportUndoLocked 把「本次导入新增的项目/节点/连线」记为一步可撤销操作：
// 拖错文件是常见误操作，一次 Ctrl+Z 应该能整体退回去（而不是逐个删设计稿）。
func (s *Store) pushImportUndoLocked(before map[string]bool) {
	var projects []*Project
	nodeIDs := make(map[string]bool, 32)
	var nodes []*Node
	var edges []*Edge
	for id, p := range s.projects {
		if before[id] {
			continue
		}
		cp := p.Clone()
		projects = append(projects, &cp)
	}
	for id, n := range s.nodes {
		for _, p := range projects {
			if n.ProjectID == p.ID {
				cp := n.Clone()
				nodes = append(nodes, &cp)
				nodeIDs[id] = true
				break
			}
		}
	}
	for _, e := range s.edges {
		if nodeIDs[e.From] && nodeIDs[e.To] {
			cp := e.Clone()
			edges = append(edges, &cp)
		}
	}
	if len(projects) == 0 {
		return
	}
	s.pushUndoLocked(undoEntry{
		desc: fmt.Sprintf("导入项目（%d 个）", len(projects)),
		undo: func() {
			for _, e := range edges {
				delete(s.edges, e.ID)
			}
			for _, n := range nodes {
				s.removeNodeLocked(n.ID)
			}
			for _, p := range projects {
				delete(s.projects, p.ID)
			}
		},
		redo: func() {
			for _, p := range projects {
				cp := *p
				s.projects[cp.ID] = &cp
			}
			for _, n := range nodes {
				s.putNodeLocked(n.Clone())
			}
			for _, e := range edges {
				cp := e.Clone()
				s.edges[cp.ID] = &cp
			}
		},
	})
}

// importProjectLocked 写入一份已校验的项目快照（调用方需持有写锁）。
func (s *Store) importProjectLocked(b ProjectBundle, cfg *ProjectCanvas, now time.Time) ProjectSummary {
	name := s.uniqueProjectNameLocked(b.Project.Name)
	pid := s.nextProjectIDLocked(now)
	p := &Project{
		ID: pid, Name: name, Note: strings.TrimSpace(b.Project.Note),
		Canvas: cfg, CreatedAt: now, UpdatedAt: now,
	}
	s.projects[pid] = p

	// 旧 ID -> 新 ID：连线按映射重建，避免指向目标实例的同名 ID
	idmap := make(map[string]string, len(b.Nodes))
	frameCount := 0
	for _, in := range b.Nodes {
		n := in.Clone()
		n.ID = s.nextNodeIDLocked(now)
		n.ProjectID = pid
		n.CreatedAt = now
		n.UpdatedAt = now
		if n.Kind == "" {
			n.Kind = KindNote
		}
		if n.HTML != "" {
			n.HTML = SanitizePreviewHTML(n.HTML)
			n.HTMLRev = now
		} else {
			n.HTMLRev = time.Time{}
		}
		if n.Width <= 0 {
			n.Width = 420
			if n.Kind == KindFrame {
				n.Width = 600
			}
		}
		if n.Height <= 0 {
			n.Height = 320
			if n.Kind == KindFrame {
				n.Height = 400
			}
		}
		if n.Z < 0 {
			n.Z = 0
		}
		if n.Z >= s.nextZ {
			s.nextZ = n.Z + 1
		}
		if n.Kind == KindFrame {
			frameCount++
		}
		if in.ID != "" {
			idmap[in.ID] = n.ID
		}
		s.putNodeLocked(n)
	}

	// 连线：两端都在本次导入内才保留；重复 / 自环直接跳过。
	pairs := make(map[string]bool, len(s.edges))
	for _, e := range s.edges {
		pairs[e.From+"\x00"+e.To] = true
		pairs[e.To+"\x00"+e.From] = true
	}
	for _, in := range b.Edges {
		from, okA := idmap[in.From]
		to, okB := idmap[in.To]
		if !okA || !okB || from == to {
			continue
		}
		if pairs[from+"\x00"+to] {
			continue
		}
		e := Edge{ID: s.nextEdgeIDLocked(now), From: from, To: to, Label: strings.TrimSpace(in.Label)}
		s.edges[e.ID] = &e
		pairs[from+"\x00"+to] = true
		pairs[to+"\x00"+from] = true
	}

	p.UpdatedAt = time.Now()
	return ProjectSummary{
		Project: p.Clone(), NodeCount: len(b.Nodes), FrameCount: frameCount,
	}
}

// uniqueProjectNameLocked 找一个不与现有项目重名的名字（大小写不敏感）。
func (s *Store) uniqueProjectNameLocked(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "导入项目"
	}
	exists := func(name string) bool {
		for _, p := range s.projects {
			if strings.EqualFold(p.Name, name) {
				return true
			}
		}
		return false
	}
	if !exists(base) {
		return base
	}
	for i := 1; i < 10000; i++ {
		name := base + "（导入）"
		if i > 1 {
			name = fmt.Sprintf("%s（导入 %d）", base, i)
		}
		if !exists(name) {
			return name
		}
	}
	return base + "（导入）"
}

func (s *Store) nextProjectIDLocked(now time.Time) string {
	for {
		s.seq++
		id := fmt.Sprintf("p-%d-%04d", now.UnixMilli(), s.seq%10000)
		if _, ok := s.projects[id]; !ok {
			return id
		}
	}
}

func (s *Store) nextNodeIDLocked(now time.Time) string {
	for {
		s.seq++
		id := fmt.Sprintf("n-%d-%04d", now.UnixMilli(), s.seq%10000)
		if _, ok := s.nodes[id]; !ok {
			return id
		}
	}
}

func (s *Store) nextEdgeIDLocked(now time.Time) string {
	for {
		s.seq++
		id := fmt.Sprintf("e-%d-%04d", now.UnixMilli(), s.seq%10000)
		if _, ok := s.edges[id]; !ok {
			return id
		}
	}
}

// ---------------- HTTP ----------------

// importEnvelope 兼容单项目 bundle 与 workspace bundle 两种请求体。
type importEnvelope struct {
	Format   string          `json:"format"`
	Version  int             `json:"version"`
	Project  *Project        `json:"project"`
	Nodes    []Node          `json:"nodes"`
	Edges    []Edge          `json:"edges"`
	Projects []ProjectBundle `json:"projects"`
}

func downloadJSON(w http.ResponseWriter, filename string, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, APIError("internal", err.Error(), ""))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
	_, _ = w.Write([]byte("\n"))
}

func (s *Server) handleExportProject(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	bundle, err := s.store.ExportProject(id)
	if err != nil {
		writeErr(w, statusForProjectErr(err), projectErr(err, id))
		return
	}
	downloadJSON(w, "adago-project-"+safeFilePart(id)+".json", bundle)
}

func (s *Server) handleExportWorkspace(w http.ResponseWriter, _ *http.Request) {
	bundle, err := s.store.ExportWorkspace()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, APIError("internal", err.Error(), ""))
		return
	}
	downloadJSON(w, "adago-workspace-"+time.Now().Format("20060102-150405")+".json", bundle)
}

func (s *Server) handleImportProjects(w http.ResponseWriter, r *http.Request) {
	var in importEnvelope
	if !decodeBody(w, r, &in) {
		return
	}
	var bundles []ProjectBundle
	switch {
	case len(in.Projects) > 0:
		if in.Format != "" && in.Format != WorkspaceBundleFormat {
			writeErr(w, http.StatusBadRequest, projectErr(fmt.Errorf("%w: %q", ErrBundleFormat, in.Format), ""))
			return
		}
		if in.Version > ProjectBundleVersion {
			writeErr(w, http.StatusBadRequest, projectErr(fmt.Errorf("%w: %d", ErrBundleVersion, in.Version), ""))
			return
		}
		bundles = in.Projects
	case in.Project != nil:
		bundles = []ProjectBundle{{
			Format: in.Format, Version: in.Version, Project: *in.Project,
			Nodes: in.Nodes, Edges: in.Edges,
		}}
	default:
		writeErr(w, http.StatusBadRequest, projectErr(fmt.Errorf("%w: 缺少 project 或 projects", ErrBundleInvalid), ""))
		return
	}
	summaries, err := s.store.ImportProjects(bundles)
	if err != nil {
		writeErr(w, http.StatusBadRequest, projectErr(err, ""))
		return
	}
	nodeCount := 0
	projects := make([]map[string]any, 0, len(summaries))
	for _, sm := range summaries {
		nodeCount += sm.NodeCount
		projects = append(projects, map[string]any{
			"id": sm.ID, "name": sm.Name, "node_count": sm.NodeCount, "frame_count": sm.FrameCount,
		})
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"imported": len(summaries), "node_count": nodeCount, "projects": projects,
	})
}

// safeFilePart 把项目 ID 收敛成安全文件名片段（只保留 ASCII 字母数字与 -_）。
func safeFilePart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "project"
	}
	return b.String()
}
