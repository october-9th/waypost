package tui

import "testing"

// Link đến từ API bên ngoài; `open` trên macOS thi hành cả scheme không phải
// web, nên phải chặn trước khi tới đó. Các case này không đẻ tiến trình nào.
func TestOpenURLRejectsNonWeb(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"mailto:a@b.c",
		"ftp://example.com",
		"/Applications/Calculator.app",
	} {
		if err := openURL(raw); err == nil {
			t.Errorf("openURL(%q) = nil, phải báo lỗi", raw)
		}
	}
}
