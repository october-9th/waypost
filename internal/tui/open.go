package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
)

func openURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("không có link")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("link hỏng: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("bỏ qua link không phải web: %s", u.Scheme)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func copyClipboard(s string) error {
	return clipboard.WriteAll(s)
}
