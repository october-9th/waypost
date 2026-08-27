package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
)

// openURL mở link bằng trình duyệt mặc định của hệ điều hành.
//
// Chỉ nhận http/https: link đến từ API bên ngoài, mà `open` trên macOS sẽ vui
// vẻ thi hành cả những scheme không phải web. Truyền URL làm argv (không qua
// shell) nên không có chuyện chèn lệnh.
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
	// Run chứ không Start: `open`/`xdg-open` trả về ngay sau khi giao việc cho
	// trình duyệt, và Run thu dọn tiến trình con thay vì để lại zombie.
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// copyClipboard chép chuỗi vào clipboard hệ thống. Dùng atotto/clipboard vì
// bubbles/textinput đã kéo nó vào binary sẵn cho tính năng paste — tự shell
// ra pbcopy nữa chỉ là làm lại việc đã có.
func copyClipboard(s string) error {
	return clipboard.WriteAll(s)
}
