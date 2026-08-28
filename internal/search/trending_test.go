package search

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func loadTrendingFixture(t *testing.T) *html.Node {
	t.Helper()
	f, err := os.Open("testdata/trending-go.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := html.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestParseTrendingRows(t *testing.T) {
	doc := loadTrendingFixture(t)
	rows := findAll(doc, func(n *html.Node) bool {
		return n.Data == "article" && strings.Contains(attr(n, "class"), "Box-row")
	})
	if len(rows) == 0 {
		t.Fatal("không thấy article.Box-row nào — parser hoặc fixture hỏng")
	}

	var got []Repo
	for _, row := range rows {
		if r, ok := parseTrendingRow(row); ok {
			got = append(got, r)
		}
	}
	if len(got) != len(rows) {
		t.Fatalf("parse được %d/%d dòng", len(got), len(rows))
	}

	first := got[0]
	if first.FullName != "agent-substrate/substrate" {
		t.Errorf("FullName = %q", first.FullName)
	}
	if first.URL != "https://github.com/agent-substrate/substrate" {
		t.Errorf("URL = %q", first.URL)
	}
	if first.Language != "Go" {
		t.Errorf("Language = %q, muốn Go", first.Language)
	}
	if first.Stars != 1632 {
		t.Errorf("Stars = %d, muốn 1632", first.Stars)
	}
	if first.StarsPeriod != 366 {
		t.Errorf("StarsPeriod = %d, muốn 366", first.StarsPeriod)
	}
	if first.Description == "" {
		t.Error("Description rỗng")
	}
	if got[1].StarsPeriod != 1926 {
		t.Errorf("dòng 2 StarsPeriod = %d, muốn 1926", got[1].StarsPeriod)
	}
}

func TestParseCount(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"1,632", 1632},
		{"\n        366 stars this week\n", 366},
		{"12", 12},
		{"", 0},
		{"không có số", 0},
	} {
		if got := parseCount(tc.in); got != tc.want {
			t.Errorf("parseCount(%q) = %d, muốn %d", tc.in, got, tc.want)
		}
	}
}

func TestTrendingLangOf(t *testing.T) {
	for _, tc := range []struct{ topic, want string }{
		{"go scheduler", "go"},
		{"rust async runtime", "rust"},
		{"golang generics", "go"},
		{"claude code memory", ""},
		{"wal internals", ""},
	} {
		if got := trendingLangOf(tc.topic); got != tc.want {
			t.Errorf("trendingLangOf(%q) = %q, muốn %q", tc.topic, got, tc.want)
		}
	}
}
