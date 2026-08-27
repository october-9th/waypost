package search

import (
	"net/url"
	"sort"
	"strings"
)

// normalizeURL đưa URL về dạng chuẩn để so trùng. Cùng một bài xuất hiện trên
// HN và lobste.rs thường khác nhau ở scheme, hoa/thường của host, www,
// trailing slash, fragment hoặc tracking param — chuẩn hóa hết để khớp key.
func normalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = "https"
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String()
}

// Merge gộp result từ nhiều nguồn theo URL đã chuẩn hóa. Bài trùng thì giữ
// điểm cao hơn và OR cờ Source lại, nên "xuất hiện trên cả HN lẫn lobste.rs"
// hiện ra ở output như một tín hiệu chất lượng nhẹ.
//
// Giữ nguyên thứ tự gặp trước — tức thứ tự relevance của nguồn. Sắp xếp là
// việc của SortResults, tách ra vì thứ tự nào đúng còn tùy topic.
func Merge(groups ...[]Result) []Result {
	byURL := make(map[string]*Result)
	order := make([]string, 0)

	for _, g := range groups {
		for _, r := range g {
			key := normalizeURL(r.URL)
			cur, ok := byURL[key]
			if !ok {
				cp := r
				byURL[key] = &cp
				order = append(order, key)
				continue
			}
			cur.Source |= r.Source
			if r.Score > cur.Score {
				cur.Score = r.Score
			}
			// Giữ title của bản đầu tiên: hai site hay đặt lại tít khác nhau,
			// không có bản nào "đúng" hơn nên khỏi cần chọn.
			if cur.Published.IsZero() || (!r.Published.IsZero() && r.Published.Before(cur.Published)) {
				cur.Published = r.Published
			}
			// CommentsURL và NumComments là một cặp — số đếm phải thuộc về
			// đúng thread đang trỏ tới, nên chỉ nhận cả cặp hoặc không nhận.
			// Caller truyền HN trước nên bài ở cả hai nguồn giữ thread HN,
			// thường là thread đông người hơn.
			if cur.CommentsURL == "" && r.CommentsURL != "" {
				cur.CommentsURL = r.CommentsURL
				cur.NumComments = r.NumComments
			}
			if cur.Description == "" {
				cur.Description = r.Description
			}
		}
	}

	out := make([]Result, 0, len(order))
	for _, k := range order {
		out = append(out, *byURL[k])
	}
	return out
}

// SortMode chọn cách xếp kết quả.
type SortMode int

const (
	// SortScore xếp giảm dần theo điểm cộng đồng. Mặc định, và đúng với topic
	// đã lắng — có đủ phiếu thì điểm chính là phán quyết.
	SortScore SortMode = iota

	// SortRelevance giữ nguyên thứ tự nguồn trả về (relevance của Algolia,
	// rồi tới lobste.rs).
	//
	// Có mode này vì đã đo (2026-08-26): với topic mới nổi, gần như mọi kết
	// quả đều 0-8 điểm. Cộng đồng CHƯA phán quyết, nên sắp theo điểm là sắp
	// theo nhiễu. Cụ thể với "claude code memory", sort theo điểm đẩy mấy bài
	// Show HN 4-5 điểm lên trên "How Claude Code memory works" — bài mà
	// Algolia xếp hạng nhất theo relevance.
	//
	// Đây KHÔNG phải công thức chấm điểm mới. Nó là chỗ thú nhận rằng lúc này
	// ta không có tín hiệu chất lượng nào, nên trả thứ tự về cho nguồn thay vì
	// giả vờ biết.
	SortRelevance
)

func (m SortMode) String() string {
	if m == SortRelevance {
		return "relevance"
	}
	return "điểm"
}

// SortResults trả về bản sao đã sắp xếp, không đụng vào slice gốc — TUI giữ
// bản relevance để đổi qua lại mà khỏi gọi lại API.
//
// Hòa điểm thì bài mới hơn lên trước, rồi đến URL, để cùng input luôn cho ra
// cùng thứ tự.
func SortResults(rs []Result, mode SortMode) []Result {
	out := make([]Result, len(rs))
	copy(out, rs)
	if mode == SortRelevance {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if !out[i].Published.Equal(out[j].Published) {
			return out[i].Published.After(out[j].Published)
		}
		return out[i].URL < out[j].URL
	})
	return out
}
