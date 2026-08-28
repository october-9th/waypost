package search

import "testing"

func scores(rs []Result) []int {
	out := make([]int, len(rs))
	for i, r := range rs {
		out[i] = r.Score
	}
	return out
}

func hits(pts ...int) []Result {
	out := make([]Result, len(pts))
	for i, p := range pts {
		out[i] = Result{URL: "https://x/" + string(rune('a'+i)), Score: p, Source: SourceHN}
	}
	return out
}

func eq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestApplyMinScore(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      []int
		min     int
		minKeep int
		want    []int
	}{
		{
			name: "cắt đuôi, giữ thứ tự relevance",
			in:   []int{559, 3, 448, 17, 2, 4, 90, 33}, min: 10, minKeep: 5,
			want: []int{559, 448, 17, 90, 33},
		},
		{
			name: "không đủ minKeep thì lấy top theo điểm",
			in:   []int{4, 1, 8, 2, 3, 1, 5}, min: 10, minKeep: 5,
			want: []int{4, 8, 2, 3, 5}, // top 5 điểm (8,5,4,3,2) trả về theo thứ tự gốc
		},
		{
			name: "qua sàn ít hơn minKeep thì kéo lại cho đủ",
			in:   []int{559, 3, 448, 2, 4}, min: 10, minKeep: 5,
			want: []int{559, 3, 448, 2, 4},
		},
		{
			name: "min=0 thì không đụng gì",
			in:   []int{5, 1, 2}, min: 0, minKeep: 5,
			want: []int{5, 1, 2},
		},
		{
			name: "ít hơn minKeep ngay từ đầu thì giữ tất",
			in:   []int{3, 1}, min: 10, minKeep: 5,
			want: []int{3, 1},
		},
		{
			name: "đúng minKeep qua sàn thì không đụng nhánh dự phòng",
			in:   []int{50, 40, 30, 20, 10, 1}, min: 10, minKeep: 5,
			want: []int{50, 40, 30, 20, 10},
		},
		{
			name: "rỗng",
			in:   nil, min: 10, minKeep: 5, want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := scores(applyMinScore(hits(tc.in...), tc.min, tc.minKeep))
			if !eq(got, tc.want) {
				t.Errorf("= %v, muốn %v", got, tc.want)
			}
		})
	}
}

func TestComposeGiuNhomRieng(t *testing.T) {
	rep := Report{
		Merged: []Result{
			{URL: "https://a", Title: "relevance đầu", Score: 4, Type: TypeVoted},
			{URL: "https://b", Title: "điểm cao", Score: 500, Type: TypeVoted},
		},
		Blog: []Result{
			{URL: "https://c", Title: "dev.to ít", Score: 9, Type: TypeBlog},
			{URL: "https://d", Title: "dev.to nhiều", Score: 38, Type: TypeBlog},
		},
		Papers: []Result{
			{URL: "https://e", Title: "paper", Score: 0, Type: TypeAcademic},
		},
	}

	got := rep.Compose(SortScore, 10)
	if len(got) != 5 {
		t.Fatalf("compose trả %d mục, muốn 5", len(got))
	}
	wantTitles := []string{"điểm cao", "relevance đầu", "dev.to nhiều", "dev.to ít", "paper"}
	for i, w := range wantTitles {
		if got[i].Title != w {
			t.Errorf("mục %d = %q, muốn %q", i, got[i].Title, w)
		}
	}

	rel := rep.Compose(SortRelevance, 10)
	if rel[0].Title != "relevance đầu" {
		t.Errorf("SortRelevance: mục 0 = %q, muốn giữ thứ tự nguồn", rel[0].Title)
	}
	if rel[2].Title != "dev.to nhiều" || rel[4].Title != "paper" {
		t.Errorf("đổi sort không được đụng nhóm phụ lục, got %q / %q", rel[2].Title, rel[4].Title)
	}
}

func TestComposeTopNChiCatNhomChinh(t *testing.T) {
	rep := Report{
		Merged: hits(100, 90, 80, 70),
		Blog:   []Result{{URL: "https://c", Score: 5, Type: TypeBlog}},
	}
	got := rep.Compose(SortScore, 2)
	if len(got) != 3 {
		t.Fatalf("= %d mục, muốn 2 voted + 1 blog", len(got))
	}
	if got[2].Type != TypeBlog {
		t.Errorf("mục cuối type = %v, muốn TypeBlog", got[2].Type)
	}
}

func TestDropSeen(t *testing.T) {
	seen := []Result{{URL: "https://blog.example.com/post/"}}
	in := []Result{
		{URL: "http://www.blog.example.com/post?utm_source=devto"}, // cùng bài
		{URL: "https://dev.to/ai/khac"},
	}
	got := dropSeen(in, seen)
	if len(got) != 1 || got[0].URL != "https://dev.to/ai/khac" {
		t.Errorf("dropSeen = %v, muốn bỏ đúng bản cross-post", scoresURL(got))
	}
}

func scoresURL(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.URL
	}
	return out
}
