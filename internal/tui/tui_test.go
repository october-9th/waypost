package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"albert/internal/search"
)

func sampleResults(n int) []search.Result {
	out := make([]search.Result, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, search.Result{
			Title:       strings.Repeat("t", 10) + string(rune('a'+i)),
			URL:         "https://example.com/a",
			Score:       100 - i,
			Source:      search.SourceHN,
			CommentsURL: "https://news.ycombinator.com/item?id=1",
			NumComments: 5,
			Published:   time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	}
	return out
}

// withResults dựng model đã có sẵn kết quả, bỏ qua đường mạng.
func withResults(t *testing.T, n, w, h int) Model {
	t.Helper()
	m := New(nil, "", 15, search.SortScore).resize(w, h)
	m.topic = "go scheduler"
	m.results = sampleResults(n)
	m.focus = focusList
	m.input.Blur()
	return m.clampWindow()
}

func key(t *testing.T, m Model, k string) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update trả về kiểu %T", next)
	}
	return got
}

func TestCursorMovesAndWindowFollows(t *testing.T) {
	m := withResults(t, 15, 80, 24)
	n := m.visibleItems()
	if n < 2 {
		t.Fatalf("visibleItems = %d, quá nhỏ với màn 24 dòng", n)
	}

	for i := 0; i < 14; i++ {
		m = key(t, m, "j")
	}
	if m.cursor[paneArticles] != 14 {
		t.Fatalf("cursor = %d, muốn 14", m.cursor[paneArticles])
	}
	if m.cursor[paneArticles] < m.offset[paneArticles] || m.cursor[paneArticles] >= m.offset[paneArticles]+n {
		t.Fatalf("con trỏ %d nằm ngoài cửa sổ [%d,%d)", m.cursor[paneArticles], m.offset[paneArticles], m.offset[paneArticles]+n)
	}

	// Đi quá cuối danh sách thì dừng ở bài cuối, không tràn.
	m = key(t, m, "j")
	if m.cursor[paneArticles] != 14 {
		t.Fatalf("cursor sau khi vượt cuối = %d, muốn 14", m.cursor[paneArticles])
	}

	m = key(t, m, "g")
	if m.cursor[paneArticles] != 0 || m.offset[paneArticles] != 0 {
		t.Fatalf("g: cursor=%d offset=%d, muốn 0/0", m.cursor[paneArticles], m.offset[paneArticles])
	}
	m = key(t, m, "k")
	if m.cursor[paneArticles] != 0 {
		t.Fatalf("k ở đầu danh sách: cursor=%d, muốn 0", m.cursor[paneArticles])
	}
}

// Gõ topic mới trong lúc topic cũ chưa xong là chuyện thường; kết quả cũ về
// trễ không được phép ghi đè kết quả đang xem.
func TestStaleResultsIgnored(t *testing.T) {
	m := withResults(t, 3, 80, 24)
	m.seq = 7
	fresh := m.results

	next, _ := m.Update(resultsMsg{seq: 6, topic: "cũ", rep: search.Report{Results: sampleResults(9)}})
	got := next.(Model)
	if len(got.results) != len(fresh) || got.topic == "cũ" {
		t.Fatalf("kết quả cũ (seq 6) đã ghi đè state của seq 7")
	}

	next, _ = m.Update(resultsMsg{seq: 7, topic: "mới", rep: search.Report{Results: sampleResults(9)}})
	got = next.(Model)
	if len(got.results) != 9 || got.topic != "mới" {
		t.Fatalf("kết quả đúng seq bị bỏ: %d bài, topic %q", len(got.results), got.topic)
	}
	if got.focus != focusList {
		t.Fatal("có kết quả thì focus phải chuyển sang danh sách")
	}
	if got.loading {
		t.Fatal("loading phải tắt sau khi có kết quả")
	}
}

func TestViewFitsHeight(t *testing.T) {
	for _, tc := range []struct{ w, h, results int }{
		{80, 24, 15},
		{120, 40, 15},
		{60, 10, 15},
		{80, 24, 0},
		{40, 24, 3},
	} {
		m := withResults(t, tc.results, tc.w, tc.h)
		lines := strings.Split(m.View(), "\n")
		if len(lines) > tc.h {
			t.Errorf("%dx%d, %d kết quả: View %d dòng, vượt màn hình", tc.w, tc.h, tc.results, len(lines))
		}
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got > tc.w {
				t.Errorf("%dx%d: dòng %d rộng %d > %d: %q", tc.w, tc.h, i, got, tc.w, ln)
			}
		}
	}
}

func TestTruncate(t *testing.T) {
	for _, tc := range []struct {
		in   string
		w    int
		want int // bề rộng tối đa mong đợi
	}{
		{"hello", 10, 5},
		{"hello world", 5, 5},
		{"日本語のタイトル", 7, 7},
		{"abc", 0, 0},
	} {
		got := truncate(tc.in, tc.w)
		if w := lipgloss.Width(got); w > tc.want {
			t.Errorf("truncate(%q, %d) = %q rộng %d, muốn ≤ %d", tc.in, tc.w, got, w, tc.want)
		}
	}
}

// `s` phải sắp lại từ merged mà KHÔNG gọi lại API (client là nil trong test —
// gọi mạng là panic ngay).
func TestDoiSortTaiCho(t *testing.T) {
	m := withResults(t, 5, 100, 30)
	m.merged = []search.Result{
		{URL: "https://a/1", Title: "relevance đầu", Score: 4, Source: search.SourceHN},
		{URL: "https://b/2", Title: "điểm cao", Score: 90, Source: search.SourceHN},
	}
	m.results = search.SortResults(m.merged, search.SortScore)

	got := key(t, m, "s")
	if got.mode != search.SortRelevance {
		t.Fatalf("mode = %v, muốn relevance", got.mode)
	}
	if got.results[0].Title != "relevance đầu" {
		t.Errorf("results[0] = %q, muốn giữ thứ tự nguồn", got.results[0].Title)
	}

	back := key(t, got, "s")
	if back.mode != search.SortScore || back.results[0].Title != "điểm cao" {
		t.Errorf("bấm s lần hai phải quay về sắp theo điểm, got %v / %q", back.mode, back.results[0].Title)
	}
}

// tab đổi pane và mỗi pane giữ con trỏ riêng.
func TestTabDoiPaneVaGiuConTro(t *testing.T) {
	m := withResults(t, 6, 100, 30)
	m.repos = []search.Repo{
		{FullName: "a/one", URL: "https://github.com/a/one", Stars: 10},
		{FullName: "b/two", URL: "https://github.com/b/two", Stars: 20},
	}
	m = key(t, m, "j")
	m = key(t, m, "j")
	if m.cursor[paneArticles] != 2 {
		t.Fatalf("cursor bài = %d, muốn 2", m.cursor[paneArticles])
	}

	m = key(t, m, "tab")
	if m.pane != paneRepos {
		t.Fatal("tab phải chuyển sang pane repo")
	}
	if m.paneLen() != 2 {
		t.Fatalf("paneLen = %d, muốn 2", m.paneLen())
	}
	m = key(t, m, "j")
	if m.cursor[paneRepos] != 1 {
		t.Fatalf("cursor repo = %d, muốn 1", m.cursor[paneRepos])
	}

	m = key(t, m, "tab")
	if m.pane != paneArticles || m.cursor[paneArticles] != 2 {
		t.Errorf("quay lại pane bài phải giữ con trỏ ở 2, got pane=%v cursor=%d", m.pane, m.cursor[paneArticles])
	}
}

// Topic mới nổi hay ra 0 bài mà vẫn có repo — phải nhảy thẳng sang pane repo.
func TestKhongCoBaiThiNhaySangRepo(t *testing.T) {
	m := New(nil, "", 15, search.SortScore).resize(100, 30)
	m.seq = 1
	rep := search.Report{Repos: []search.Repo{{FullName: "a/one", URL: "https://github.com/a/one"}}}
	next, _ := m.Update(resultsMsg{seq: 1, topic: "claude code memory", rep: rep})
	got := next.(Model)
	if got.pane != paneRepos {
		t.Fatalf("pane = %v, muốn repo khi không có bài nào", got.pane)
	}
	if got.focus != focusList {
		t.Error("focus phải sang danh sách để j/k dùng được ngay")
	}
}
