package tui

import "testing"

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
