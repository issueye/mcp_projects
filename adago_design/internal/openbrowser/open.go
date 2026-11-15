// Package openbrowser 以系统默认浏览器打开画布地址（原生窗口的回退路径）。
package openbrowser

import (
	"os"
	"os/exec"
	"runtime"
)

// Open 尝试用默认浏览器打开 url，返回启动失败的原因（不阻塞、不等浏览器退出）。
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// start 会把带 & 的 URL 截断，包一层 cmd /c start "" "url"
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if v := os.Getenv("ADAGO_DESIGN_BROWSER"); v != "" {
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", "", v, url)
		case "darwin":
			cmd = exec.Command("open", "-a", v, url)
		default:
			cmd = exec.Command(v, url)
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
