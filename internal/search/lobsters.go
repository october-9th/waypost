package search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// lobstersTagPages là số trang lấy cho mỗi lần search. Mỗi trang 25 story theo
// thứ tự mới nhất, nên 3 trang ≈ 75 bài, trải khoảng 1-2 tháng với tag đông.
// Đừng tăng bừa: đây là site nhỏ tự host, mỗi trang là một request.
const lobstersTagPages = 3

// maxTags là số tag tối đa map từ một topic. Lấy nhiều hơn thì tag khớp yếu
// sẽ kéo vào toàn bài lạc đề.
const maxTags = 2

// tagsCacheTTL — tập tag của lobste.rs gần như không đổi (116 tag), cache dài
// tay được. Cache miss cũng chỉ tốn một request, nên không cần TTL ngắn.
const tagsCacheTTL = 7 * 24 * time.Hour

// tagAliases lấp chỗ mà tên tag lẫn description đều không khớp topic thường
// gặp. Cố tình để nhỏ: mỗi dòng phải là thứ thật sự hay gõ, không đoán trước.
var tagAliases = map[string]string{
	"sqlite":     "databases",
	"postgres":   "databases",
	"postgresql": "databases",
	"mysql":      "databases",
	"redis":      "databases",
	"sql":        "databases",
	"kubernetes": "devops",
	"k8s":        "devops",
	"docker":     "devops",
	"terraform":  "devops",
	"llm":        "ai",
	"llms":       "ai",
	"golang":     "go",
}

// Tag là một tag trên lobste.rs. Description hay chứa tên gọi khác của tag
// ("Golang programming" cho tag `go`) nên cũng dùng để so khớp topic.
type Tag struct {
	Tag         string `json:"tag"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

type lobstersStory struct {
	Title            string   `json:"title"`
	URL              string   `json:"url"`
	Score            int      `json:"score"`
	Tags             []string `json:"tags"`
	CreatedAt        string   `json:"created_at"`
	CommentsURL      string   `json:"comments_url"`
	CommentCount     int      `json:"comment_count"`
	DescriptionPlain string   `json:"description_plain"`
}

// Tags trả về danh sách tag của lobste.rs, ưu tiên đọc từ cache trên đĩa.
// Lỗi cache không làm hỏng lời gọi — cùng lắm là gọi lại API.
func (c *Client) Tags(ctx context.Context) ([]Tag, error) {
	if tags, ok := c.readTagsCache(); ok {
		return tags, nil
	}

	var tags []Tag
	if err := c.getJSON(ctx, "https://lobste.rs/tags.json", &tags); err != nil {
		return nil, fmt.Errorf("lobste.rs tags: %w", err)
	}
	c.writeTagsCache(tags)
	return tags, nil
}

func (c *Client) tagsCachePath() string {
	if c.cacheDir == "" {
		return ""
	}
	return filepath.Join(c.cacheDir, "lobsters-tags.json")
}

func (c *Client) readTagsCache() ([]Tag, bool) {
	path := c.tagsCachePath()
	if path == "" {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > tagsCacheTTL {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var tags []Tag
	if err := json.Unmarshal(data, &tags); err != nil || len(tags) == 0 {
		return nil, false
	}
	return tags, true
}

func (c *Client) writeTagsCache(tags []Tag) {
	path := c.tagsCachePath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return
	}
	// Ghi tạm rồi rename để lần chạy sau không đọc phải file ghi dở.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
	}
}

// matchTags map topic sang tag của lobste.rs.
//
// Trả về tag đã chọn và extra: các token của topic không tag nào "nuốt" được.
// extra chính là phần topic hẹp hơn tag (vd "go scheduler" → tag `go`, extra
// ["scheduler"]) và sẽ dùng để lọc title. extra rỗng nghĩa là topic trùng
// khít tag, cả feed đều đúng chủ đề, khỏi lọc.
//
// Trả về tag rỗng nghĩa là không map được — khi đó bỏ hẳn lobste.rs thay vì
// đoán bừa một tag.
func matchTags(topic string, tags []Tag) (picked []string, extra []string) {
	// Chọn tag bằng token đã lọc từ chung; tính extra bằng token đầy đủ, để
	// từ như "design" vẫn dùng được cho bước lọc title dù vô dụng khi chọn tag.
	tokens := tagTokens(topic)
	if len(tokens) == 0 {
		return nil, nil
	}

	type scored struct {
		tag      string
		points   int
		consumed []string
	}
	var matches []scored

	for _, t := range tags {
		if !t.Active {
			continue
		}
		descTokens := tokenize(t.Description)
		var points int
		var consumed []string
		for _, tok := range tokens {
			switch {
			case stem(tok) == stem(t.Tag):
				// Khớp thẳng tên tag là tín hiệu mạnh nhất.
				points += 2 * len(tok)
				consumed = append(consumed, tok)
			case tagAliases[tok] == t.Tag:
				points += 2 * len(tok)
				consumed = append(consumed, tok)
			case matchesAny(t.Description, []string{tok}) && len(descTokens) > 0:
				points += len(tok)
				consumed = append(consumed, tok)
			}
		}
		if points > 0 {
			matches = append(matches, scored{t.Tag, points, consumed})
		}
	}

	if len(matches) == 0 {
		return nil, nil
	}

	// Điểm cao trước; hòa thì theo tên tag để cùng topic luôn ra cùng tag.
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0; j-- {
			if matches[j].points > matches[j-1].points ||
				(matches[j].points == matches[j-1].points && matches[j].tag < matches[j-1].tag) {
				matches[j], matches[j-1] = matches[j-1], matches[j]
				continue
			}
			break
		}
	}
	if len(matches) > maxTags {
		matches = matches[:maxTags]
	}

	consumed := make(map[string]bool)
	for _, m := range matches {
		picked = append(picked, m.tag)
		for _, tok := range m.consumed {
			consumed[tok] = true
		}
	}
	for _, tok := range tokenize(topic) {
		if !consumed[tok] {
			extra = append(extra, tok)
		}
	}
	return picked, extra
}

// SearchLobsters tìm bài trên lobste.rs qua tag map. Trả về nil (không lỗi)
// khi topic không khớp tag nào — đó là kết quả hợp lệ, không phải hỏng hóc;
// caller chạy tiếp với mỗi HN.
func (c *Client) SearchLobsters(ctx context.Context, topic string) ([]Result, []string, error) {
	topic = NormalizeTopic(topic)
	if topic == "" {
		return nil, nil, nil
	}

	tags, err := c.Tags(ctx)
	if err != nil {
		return nil, nil, err
	}

	picked, extra := matchTags(topic, tags)
	if len(picked) == 0 {
		return nil, nil, nil
	}

	stories, err := c.fetchTagPages(ctx, picked, lobstersTagPages)
	if err != nil {
		return nil, picked, err
	}

	out := make([]Result, 0, len(stories))
	for _, s := range stories {
		if s.URL == "" || s.Title == "" {
			continue
		}
		// Topic hẹp hơn tag → bài phải nhắc tới phần hẹp đó ở title hoặc tag
		// riêng của nó. matchesAny trả true khi extra rỗng, tức không lọc.
		if !matchesAny(s.Title+" "+strings.Join(s.Tags, " "), extra) {
			continue
		}
		r := Result{
			Title:       s.Title,
			URL:         s.URL,
			Score:       s.Score,
			Source:      SourceLobsters,
			CommentsURL: s.CommentsURL,
			NumComments: s.CommentCount,
			Description: strings.TrimSpace(s.DescriptionPlain),
		}
		if t, err := time.Parse(time.RFC3339, s.CreatedAt); err == nil {
			r.Published = t.UTC()
		}
		out = append(out, r)
	}
	return out, picked, nil
}

// fetchTagPages lấy nhiều trang của một (hoặc vài) tag. Endpoint nhiều tag
// `/t/a,b.json` là hợp của hai tag nên chỉ tốn một request mỗi trang.
//
// Gọi tuần tự và nghỉ giữa các trang: lobste.rs tự host, đừng bắn song song.
func (c *Client) fetchTagPages(ctx context.Context, tags []string, pages int) ([]lobstersStory, error) {
	joined := strings.Join(tags, ",")
	var all []lobstersStory

	for page := 1; page <= pages; page++ {
		// Trang 1 dùng `/t/{tag}.json`; từ trang 2 là `/t/{tag}/page/{n}.json`.
		// Dạng `?page=n` KHÔNG hoạt động — server bỏ qua và trả lại trang 1.
		endpoint := fmt.Sprintf("https://lobste.rs/t/%s.json", joined)
		if page > 1 {
			endpoint = fmt.Sprintf("https://lobste.rs/t/%s/page/%d.json", joined, page)
		}

		var stories []lobstersStory
		if err := c.getJSON(ctx, endpoint, &stories); err != nil {
			if len(all) > 0 {
				// Đã có dữ liệu trang trước thì dùng tạm, không vứt hết.
				return all, nil
			}
			return nil, fmt.Errorf("lobste.rs tag %q: %w", joined, err)
		}
		all = append(all, stories...)
		if len(stories) == 0 {
			break // hết bài, khỏi xin trang tiếp
		}

		if page < pages {
			select {
			case <-ctx.Done():
				return all, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	return all, nil
}
