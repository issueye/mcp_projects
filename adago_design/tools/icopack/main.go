// Command icopack 从 assets/icon.png（512×512 母版）生成：
//   - assets/icon.ico         exe 图标（256/128/64/48/32/16，PNG 编码）
//   - internal/native/assets/icon.png   窗口图标（128）
//   - internal/native/assets/tray.png   托盘图标（32）
//
// 用法（模块根目录）：
//
//	GOWORK=off go run ./tools/icopack
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

var sizes = []int{256, 128, 64, 48, 32, 16}

type entry struct {
	size int
	data []byte
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	src, err := os.Open(filepath.Join(root, "assets", "icon.png"))
	if err != nil {
		fatal(err)
	}
	img, err := png.Decode(src)
	_ = src.Close()
	if err != nil {
		fatal(err)
	}

	// ---- .ico ----
	var entries []entry
	for _, size := range sizes {
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		var b bytes.Buffer
		if err := png.Encode(&b, dst); err != nil {
			fatal(err)
		}
		entries = append(entries, entry{size: size, data: b.Bytes()})
	}

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // icon
	_ = binary.Write(&out, binary.LittleEndian, uint16(len(entries)))
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		w := byte(e.size)
		if e.size == 256 {
			w = 0 // 256 在 ICO 里写 0
		}
		out.Write([]byte{w, w, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))  // planes
		_ = binary.Write(&out, binary.LittleEndian, uint16(32)) // bits per pixel
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(e.data)))
		_ = binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(e.data)
	}
	for _, e := range entries {
		out.Write(e.data)
	}
	icoPath := filepath.Join(root, "assets", "icon.ico")
	if err := os.WriteFile(icoPath, out.Bytes(), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes, %d sizes)\n", icoPath, out.Len(), len(entries))

	// ---- 窗口 / 托盘 PNG ----
	writePNG := func(size int, rel string) {
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		var b bytes.Buffer
		if err := png.Encode(&b, dst); err != nil {
			fatal(err)
		}
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote %s (%d bytes, %d×%d)\n", path, b.Len(), size, size)
	}
	writePNG(128, filepath.Join("internal", "native", "assets", "icon.png"))
	writePNG(32, filepath.Join("internal", "native", "assets", "tray.png"))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "icopack:", err)
	os.Exit(1)
}
