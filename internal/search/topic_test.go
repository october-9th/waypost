package search

import (
	"reflect"
	"testing"
)

func TestStemHoiTuVeMotGoc(t *testing.T) {
	groups := [][]string{
		{"scheduler", "scheduling", "schedules", "schedule"},
		{"database", "databases"},
		{"compiler", "compilers", "compiling", "compile"},
		{"cache", "caches", "caching"},
		{"test", "tests", "testing"},
	}
	for _, g := range groups {
		want := stem(g[0])
		for _, w := range g[1:] {
			if got := stem(w); got != want {
				t.Errorf("stem(%q) = %q, muốn %q (cùng gốc với %q)", w, got, want, g[0])
			}
		}
	}
}

func TestStemGiuNguyenTokenNgan(t *testing.T) {
	// Cắt hậu tố token ngắn sẽ phá tên công nghệ: "css" → "c", "ios" → "i".
	for _, tok := range []string{"go", "c", "css", "ios", "wal", "sql", "zig", "rust"} {
		if got := stem(tok); got != tok {
			t.Errorf("stem(%q) = %q, muốn giữ nguyên", tok, got)
		}
	}
}

func TestTokenizeGiuKyTuTenNgonNgu(t *testing.T) {
	// '+' và '#' là một phần của tên ngôn ngữ, cắt đi là mất tag `c++`.
	got := tokenize("C++ vs C# programming")
	want := []string{"c++", "c#", "programming"} // "vs" là từ nối, bỏ
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenize = %v, muốn %v", got, want)
	}
}

func TestTagTokensBoTuChungChung(t *testing.T) {
	// tagTokens dùng để chọn tag nên phải bỏ từ chung; tokenize dùng để lọc
	// title nên giữ lại. Trộn hai cái là mất khả năng lọc bài lạc đề.
	if got, want := tagTokens("compiler design"), []string{"compiler"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tagTokens = %v, muốn %v", got, want)
	}
	if got, want := tokenize("compiler design"), []string{"compiler", "design"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tokenize = %v, muốn %v", got, want)
	}
}

func TestMatchesAnyRongThiKhongLoc(t *testing.T) {
	if !matchesAny("bất kỳ tiêu đề nào", nil) {
		t.Error("token rỗng phải cho qua tất cả, không được lọc sạch")
	}
}

func TestMatchesAnyKhongKhopTuChungPhu(t *testing.T) {
	// "going" không được coi là "go" — nếu stem cắt bừa thì mọi bài có chữ
	// "going" sẽ lọt vào kết quả topic Go.
	if matchesAny("Going freestanding", []string{"go"}) {
		t.Error(`"Going freestanding" không được khớp token "go"`)
	}
	if !matchesAny("Understanding the Go Scheduler", []string{"go"}) {
		t.Error(`title có chữ "Go" phải khớp token "go"`)
	}
}

// tagFixture là tập con tag thật của lobste.rs (verify 2026-08-26).
var tagFixture = []Tag{
	{Tag: "go", Description: "Golang programming", Active: true},
	{Tag: "rust", Description: "Rust programming", Active: true},
	{Tag: "databases", Description: "Databases (SQL, NoSQL)", Active: true},
	{Tag: "distributed", Description: "Distributed systems", Active: true},
	{Tag: "elixir", Description: "Elixir programming", Active: true},
	{Tag: "gleam", Description: "Strongly-typed BEAM language", Active: true},
	{Tag: "plt", Description: "Programming language theory, types, design", Active: true},
	{Tag: "compilers", Description: "Compiler design", Active: true},
	{Tag: "ml", Description: "MetaLanguage, OCaml programming", Active: true},
	{Tag: "vim", Description: "Vim editor", Active: false},
}

func TestMatchTags(t *testing.T) {
	tests := []struct {
		name      string
		topic     string
		wantTags  []string
		wantExtra []string
	}{
		{"tên tag khớp thẳng", "go scheduler", []string{"go"}, []string{"scheduler"}},
		{"topic trùng khít tag thì không lọc", "rust", []string{"rust"}, nil},
		{"alias cho tên không có trong tag", "sqlite wal internals", []string{"databases"}, []string{"wal", "internals"}},
		{"golang là alias của go", "golang generics", []string{"go"}, []string{"generics"}},
		{"khớp qua description", "beam language", []string{"gleam"}, []string{"language"}},
		{"từ chung không chọn tag nhưng vẫn lọc title", "compiler design", []string{"compilers"}, []string{"design"}},
		{"tối đa 2 tag", "elixir beam scheduler", []string{"elixir", "gleam"}, []string{"scheduler"}},
		{"không khớp gì thì bỏ lobste.rs", "quantum knitting patterns", nil, nil},
		{"tag inactive bị bỏ qua", "vim", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTags, gotExtra := matchTags(tt.topic, tagFixture)
			if !reflect.DeepEqual(gotTags, tt.wantTags) {
				t.Errorf("tag = %v, muốn %v", gotTags, tt.wantTags)
			}
			if !reflect.DeepEqual(gotExtra, tt.wantExtra) {
				t.Errorf("extra = %v, muốn %v", gotExtra, tt.wantExtra)
			}
		})
	}
}

func TestMatchTagsKhongVuotQuaMaxTags(t *testing.T) {
	tags, _ := matchTags("go rust elixir databases distributed", tagFixture)
	if len(tags) > maxTags {
		t.Errorf("matchTags trả %d tag (%v), tối đa %d", len(tags), tags, maxTags)
	}
}
