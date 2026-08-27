// Package search tìm bài viết kỹ thuật theo topic từ Hacker News và lobste.rs,
// dùng điểm upvote của cộng đồng làm tín hiệu chất lượng thay vì tự chấm điểm.
package search

import (
	"strings"
	"time"
)

// Source là bitmask nguồn của một Result. Dùng bitmask vì sau khi merge, một
// bài có thể đến từ cả hai nguồn — bản thân việc đó đã là tín hiệu chất lượng.
type Source uint8

const (
	SourceHN Source = 1 << iota
	SourceLobsters
)

// MarshalJSON in Source ra chuỗi ("HN+Lobsters") thay vì số bitmask — output
// -json là để người đọc và để pipe sang jq, số 3 không nói lên điều gì.
func (s Source) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

func (s Source) String() string {
	switch s {
	case SourceHN:
		return "HN"
	case SourceLobsters:
		return "Lobsters"
	case SourceHN | SourceLobsters:
		return "HN+Lobsters"
	default:
		return "?"
	}
}

// Result là một bài viết đã chuẩn hóa từ một trong hai nguồn.
type Result struct {
	Title string `json:"title"`
	URL   string `json:"url"`

	// Score là điểm cộng đồng gốc: points của HN, score của lobste.rs. Sau khi
	// merge hai nguồn thì lấy max — không cộng dồn, vì thang điểm hai site khác
	// nhau (HN đông hơn lobste.rs cả chục lần) nên cộng lại không có nghĩa gì.
	Score int `json:"score"`

	Source    Source    `json:"source"`
	Published time.Time `json:"published"`

	// CommentsURL là trang thảo luận của bài (thread HN hoặc lobste.rs). Với
	// nhiều bài, thread đáng mở ngang bài gốc — TUI cho mở riêng bằng phím `c`.
	CommentsURL string `json:"comments_url,omitempty"`
	NumComments int    `json:"num_comments,omitempty"`

	// Description là lời giới thiệu người submit tự viết. Chỉ lobste.rs có;
	// HN không có gì tương đương nên phần lớn Result để trống.
	Description string `json:"description,omitempty"`

	// ShowHN đánh dấu bài "Show HN" — người ta khoe thứ vừa build, không phải
	// bài viết để đọc. KHÔNG lọc bỏ, vì với câu hỏi "đã có ai build cái này
	// chưa" thì Show HN chính là câu trả lời. Chỉ đánh dấu để phân biệt bằng
	// mắt giữa "sản phẩm" và "bài viết".
	ShowHN bool `json:"show_hn,omitempty"`
}

// grammarWords là từ nối thuần ngữ pháp — bỏ ở mọi nơi vì không mang thông
// tin gì, kể cả khi lọc title.
var grammarWords = map[string]bool{
	"and": true, "or": true, "the": true, "a": true, "an": true, "to": true,
	"for": true, "with": true, "in": true, "of": true, "on": true, "not": true,
	"when": true, "about": true, "vs": true,
}

// genericTagWords là từ chung chung xuất hiện trong quá nhiều description tag
// của lobste.rs nên không phân biệt được gì: "Ruby programming", "Go
// programming"... Không loại thì topic nào chứa "programming" cũng khớp toàn
// bộ tag ngôn ngữ, "beam language" khớp luôn `plt` ("Programming language
// theory"), "compiler design" khớp luôn `design` (thực ra là visual design).
//
// CHỈ loại khi map topic sang tag, KHÔNG loại khi lọc title: với "compiler
// design" thì "design" vô dụng để chọn tag nhưng lại là từ khóa hữu ích để
// loại bài lạc đề trong feed tag `compilers`.
//
// Có tên tag trùng trong danh sách này cũng không sao — khớp tên tag so trực
// tiếp với chuỗi tag, không đi qua bước lọc từ.
var genericTagWords = map[string]bool{
	"programming": true, "development": true, "other": true, "link": true,
	"related": true, "stories": true, "use": true, "language": true,
	"languages": true, "system": true, "systems": true, "design": true,
	"editor": true,
}

// NormalizeTopic chuẩn hóa topic người dùng nhập: bỏ khoảng trắng thừa, hạ về
// chữ thường. Dùng chung cho query và cache key, để "Go Scheduler" và
// "go  scheduler" không thành hai thứ khác nhau.
func NormalizeTopic(topic string) string {
	return strings.Join(strings.Fields(strings.ToLower(topic)), " ")
}

// tokenize cắt chuỗi thành các token chữ-số, giữ lại '+' và '#' vì chúng là
// một phần của tên ngôn ngữ (c++, c#, f#) và cũng là tên tag trên lobste.rs.
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

// tagTokens là token của topic dùng để map sang tag lobste.rs: giống tokenize
// nhưng bỏ thêm từ chung chung. Xem genericTagWords.
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

// stem cắt hậu tố biến cách đơn giản để "scheduler", "scheduling", "schedules",
// "schedule" cùng quy về một gốc ("schedul"). Cắt cả "e" cuối là có chủ đích:
// không có nó thì "database" và "databases" ra hai gốc khác nhau.
// Không đụng token ngắn (<5 ký tự) để khỏi phá "go", "os", "ios", "css".
// Đây là stemmer thô có chủ đích — đủ để so khớp title, không cần Porter.
func stem(tok string) string {
	if len(tok) < 5 {
		return tok
	}
	// Thứ tự dài trước ngắn sau: "ers" phải xét trước "er", "es" trước "s".
	for _, suf := range []string{"ing", "ers", "er", "es", "s", "e"} {
		if len(tok)-len(suf) >= 4 && strings.HasSuffix(tok, suf) {
			return tok[:len(tok)-len(suf)]
		}
	}
	return tok
}

// matchesAny cho biết text có chứa ít nhất một trong các token (so theo gốc từ).
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
