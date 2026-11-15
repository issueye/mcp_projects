// Package applog 统一日志落点：数据目录 + stderr。
//
// windowsgui 构建（无控制台）里 stderr 不可用，写失败会被忽略；文件始终保留，
// 所以双击启动或用 GUI 子系统跑 `serve` 时，排障仍能拿到 <data>/adago-design.log。
package applog

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// maxSize 触发一次简单轮换（旧文件改名为 .1，只留一份历史）。
const maxSize = 4 << 20

// Setup 把标准日志接到 <dataDir>/adago-design.log 与 stderr。
// 打不开文件时保持原样（stderr），不影响启动。
func Setup(dataDir string) {
	if dataDir == "" {
		return
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dataDir, "adago-design.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() >= maxSize {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(tee{file: f, console: os.Stderr})
}

// tee 写两个目标；任一失败都不影响另一个，也不向上返回错误
// （log 包对写错误只在调试时关心，windowsgui 下 stderr 必然失败）。
type tee struct {
	file    io.Writer
	console io.Writer
}

func (t tee) Write(p []byte) (int, error) {
	if t.console != nil {
		_, _ = t.console.Write(p)
	}
	if t.file != nil {
		_, _ = t.file.Write(p)
	}
	return len(p), nil
}
