package search

import (
	"testing"
	"time"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"http vs https", "http://go.dev/blog/scheduler", "https://go.dev/blog/scheduler"},
		{"host hoa thường", "https://Go.Dev/blog/scheduler", "https://go.dev/blog/scheduler"},
		{"www", "https://www.go.dev/blog/scheduler", "https://go.dev/blog/scheduler"},
		{"trailing slash", "https://go.dev/blog/scheduler/", "https://go.dev/blog/scheduler"},
		{"fragment", "https://go.dev/blog/scheduler#gmp", "https://go.dev/blog/scheduler"},
		{"utm params", "https://go.dev/blog/scheduler?utm_source=hn", "https://go.dev/blog/scheduler"},
		{"path phân biệt hoa thường", "https://go.dev/Blog", "https://go.dev/Blog"},
		{"url hỏng giữ nguyên", "://not a url", "://not a url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeURL(tt.raw); got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, muốn %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMergeGopChungNguon(t *testing.T) {
	hn := []Result{
		{URL: "https://go.dev/blog/scheduler", Title: "Scheduler", Score: 120, Source: SourceHN},
		{URL: "https://ardanlabs.com/a", Title: "Chỉ HN", Score: 300, Source: SourceHN},
	}
	lob := []Result{
		{URL: "http://www.go.dev/blog/scheduler/", Title: "Scheduler (lob)", Score: 45, Source: SourceLobsters},
		{URL: "https://lobste.rs-only.example/b", Title: "Chỉ Lobsters", Score: 200, Source: SourceLobsters},
	}

	got := Merge(hn, lob)

	if len(got) != 3 {
		t.Fatalf("Merge trả về %d result, muốn 3: %+v", len(got), got)
	}

	var merged *Result
	for i := range got {
		if got[i].URL == "https://go.dev/blog/scheduler" {
			merged = &got[i]
		}
	}
	if merged == nil {
		t.Fatal("không tìm thấy bài đã gộp trong output")
	}
	if merged.Source != SourceHN|SourceLobsters {
		t.Errorf("Source = %v, muốn HN+Lobsters", merged.Source)
	}
	if merged.Score != 120 {
		t.Errorf("Score = %d, muốn 120 (max của hai nguồn, không phải tổng)", merged.Score)
	}
	if merged.Title != "Scheduler" {
		t.Errorf("Title = %q, muốn giữ title của bản gặp trước", merged.Title)
	}
}

func TestMergeSortGiamDanTheoScore(t *testing.T) {
	got := SortResults(Merge([]Result{
		{URL: "https://a.example/1", Score: 10, Source: SourceHN},
		{URL: "https://b.example/2", Score: 300, Source: SourceHN},
		{URL: "https://c.example/3", Score: 55, Source: SourceHN},
	}), SortScore)

	want := []int{300, 55, 10}
	for i, w := range want {
		if got[i].Score != w {
			t.Errorf("got[%d].Score = %d, muốn %d", i, got[i].Score, w)
		}
	}
}

func TestMergeHoaDiemThiBaiMoiHonLenTruoc(t *testing.T) {
	cu := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	moi := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := SortResults(Merge([]Result{
		{URL: "https://a.example/cu", Score: 50, Published: cu, Source: SourceHN},
		{URL: "https://b.example/moi", Score: 50, Published: moi, Source: SourceHN},
	}), SortScore)

	if got[0].URL != "https://b.example/moi" {
		t.Errorf("got[0].URL = %q, muốn bài mới hơn lên trước", got[0].URL)
	}
}

func TestSortRelevanceGiuNguyenThuTuNguon(t *testing.T) {
	in := []Result{
		{URL: "https://a.example/1", Score: 4, Source: SourceHN},
		{URL: "https://b.example/2", Score: 0, Source: SourceHN},
		{URL: "https://c.example/3", Score: 8, Source: SourceHN},
	}
	got := SortResults(in, SortRelevance)
	for i, want := range []string{"https://a.example/1", "https://b.example/2", "https://c.example/3"} {
		if got[i].URL != want {
			t.Errorf("got[%d].URL = %q, muốn %q", i, got[i].URL, want)
		}
	}
	if in[0].URL != "https://a.example/1" {
		t.Error("SortResults sửa slice gốc")
	}
}
