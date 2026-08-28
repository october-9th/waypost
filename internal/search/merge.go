package search

import (
	"net/url"
	"sort"
	"strings"
)

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

// Merge gộp kết quả trùng URL và giữ nguyên thứ tự đầu vào.
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
			if cur.Published.IsZero() || (!r.Published.IsZero() && r.Published.Before(cur.Published)) {
				cur.Published = r.Published
			}
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

func dropSeen(rs []Result, seen []Result) []Result {
	if len(rs) == 0 || len(seen) == 0 {
		return rs
	}
	keys := make(map[string]bool, len(seen))
	for _, r := range seen {
		keys[normalizeURL(r.URL)] = true
	}
	out := make([]Result, 0, len(rs))
	for _, r := range rs {
		if !keys[normalizeURL(r.URL)] {
			out = append(out, r)
		}
	}
	return out
}

// SortMode chọn cách xếp kết quả.
type SortMode int

const (
	// SortScore xếp theo điểm cộng đồng giảm dần.
	SortScore SortMode = iota

	SortRelevance
)

func (m SortMode) String() string {
	if m == SortRelevance {
		return "relevance"
	}
	return "điểm"
}

// SortResults trả về bản sao đã sắp xếp.
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
