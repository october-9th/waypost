// Package search tìm và xếp hạng nội dung kỹ thuật từ nhiều nguồn.
package search

import (
	"strings"
	"time"
)

// Source là bitmask nguồn của một Result.
type Source uint8

const (
	SourceHN Source = 1 << iota
	SourceLobsters
	SourceDevTo
	SourceArXiv
)

var sourceNames = []struct {
	bit  Source
	name string
}{
	{SourceHN, "HN"},
	{SourceLobsters, "Lobsters"},
	{SourceDevTo, "dev.to"},
	{SourceArXiv, "arXiv"},
}

// MarshalJSON mã hóa Source thành nhãn dễ đọc.
func (s Source) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

func (s Source) String() string {
	var parts []string
	for _, sn := range sourceNames {
		if s&sn.bit != 0 {
			parts = append(parts, sn.name)
		}
	}
	if len(parts) == 0 {
		return "?"
	}
	return strings.Join(parts, "+")
}

// ContentType nhóm các kết quả có thang điểm tương đương.
type ContentType uint8

const (
	// TypeVoted gồm kết quả HN và lobste.rs.
	TypeVoted ContentType = iota

	TypeBlog

	TypeAcademic
)

func (t ContentType) String() string {
	switch t {
	case TypeBlog:
		return "blog"
	case TypeAcademic:
		return "paper"
	default:
		return "voted"
	}
}

func (t ContentType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

// Result là một bài viết đã được chuẩn hóa.
type Result struct {
	Title string `json:"title"`
	URL   string `json:"url"`

	// Score là điểm cộng đồng gốc của nguồn.
	Score int `json:"score"`

	Source    Source      `json:"source"`
	Type      ContentType `json:"type"`
	Published time.Time   `json:"published"`

	CommentsURL string `json:"comments_url,omitempty"`
	NumComments int    `json:"num_comments,omitempty"`

	Description string `json:"description,omitempty"`

	ShowHN bool `json:"show_hn,omitempty"`
}

var grammarWords = map[string]bool{
	"and": true, "or": true, "the": true, "a": true, "an": true, "to": true,
	"for": true, "with": true, "in": true, "of": true, "on": true, "not": true,
	"when": true, "about": true, "vs": true,
}

var genericTagWords = map[string]bool{
	"programming": true, "development": true, "other": true, "link": true,
	"related": true, "stories": true, "use": true, "language": true,
	"languages": true, "system": true, "systems": true, "design": true,
	"editor": true,
}

func NormalizeTopic(topic string) string {
	return strings.Join(strings.Fields(strings.ToLower(topic)), " ")
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '+', r == '#':
			return false
		default:
			return true
		}
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" && !grammarWords[f] {
			out = append(out, f)
		}
	}
	return out
}

func tagTokens(s string) []string {
	toks := tokenize(s)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		if !genericTagWords[t] {
			out = append(out, t)
		}
	}
	return out
}

func stem(tok string) string {
	if len(tok) < 5 {
		return tok
	}
	for _, suf := range []string{"ing", "ers", "er", "es", "s", "e"} {
		if len(tok)-len(suf) >= 4 && strings.HasSuffix(tok, suf) {
			return tok[:len(tok)-len(suf)]
		}
	}
	return tok
}

func matchesAll(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	have := make(map[string]bool)
	for _, w := range tokenize(text) {
		have[stem(w)] = true
	}
	for _, t := range tokens {
		if !have[stem(t)] {
			return false
		}
	}
	return true
}

func matchesAny(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	stems := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		stems[stem(t)] = true
	}
	for _, w := range tokenize(text) {
		if stems[stem(w)] {
			return true
		}
	}
	return false
}
