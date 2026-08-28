package search

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// agentProducts là tên sản phẩm coding agent.
//
// Repo trong lĩnh vực này gần như luôn tự mô tả bằng CƠ CHẾ, không bằng tên
// sản phẩm — `thedotmack/claude-mem` (92k sao) mô tả mình là "Persistent
// Context Across Sessions for Every Agent", trong đó không có chữ "memory".
// Hệ quả đo được (2026-08-28): query `claude code memory` KHÔNG trả về
// claude-mem, mem0 hay cognee trong **100 kết quả đầu**; đổi sang query theo
// cơ chế `agent memory` thì mem0 ra hạng 2, cognee hạng 19.
//
// Đây chính là quy tắc "search bằng cơ chế, đừng search bằng tên term" đã ghi
// trong plan — trước đây bắt người dùng tự nhớ và tự gõ, giờ để code tự làm.
var agentProducts = map[string]bool{
	"claude": true, "anthropic": true, "codex": true, "openai": true,
	"chatgpt": true, "gpt": true, "cursor": true, "copilot": true,
	"gemini": true, "windsurf": true, "opencode": true, "aider": true,
}

// mechanismQuery đổi query chứa tên sản phẩm thành query theo cơ chế.
// "claude code memory" → "agent memory". Trả rỗng khi topic không chứa tên
// sản phẩm nào — khi đó không bắn thêm request, giữ nguyên 1 request/lần tìm
// (search API của GitHub chỉ cho 10 request/phút khi không có token).
func mechanismQuery(topic string) string {
	var kept []string
	var sawProduct bool
	for _, tok := range tokenize(topic) {
		switch {
		case agentProducts[tok]:
			sawProduct = true
		case tok == "code" || tok == "coding":
			// "code" đi kèm tên sản phẩm ("claude code") là một phần của tên,
			// không phải cơ chế. Bỏ luôn, nếu không "code memory" vẫn trượt.
			sawProduct = true
		default:
			kept = append(kept, tok)
		}
	}
	if !sawProduct || len(kept) == 0 {
		return ""
	}
	// "agent" thay chỗ tên sản phẩm vừa bỏ: đó là từ mà chính các repo này
	// dùng để tự mô tả.
	return "agent " + strings.Join(kept, " ")
}

const ghPerPage = 15

// Repo chứa metadata của một GitHub repository.
type Repo struct {
	FullName    string    `json:"full_name"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Stars       int       `json:"stars"`
	Pushed      time.Time `json:"pushed"`
	Archived    bool      `json:"archived"`

	StarsPeriod int    `json:"stars_period,omitempty"`
	Period      string `json:"period,omitempty"`

	Language string `json:"language,omitempty"`
}

type ghResponse struct {
	Items []ghItem `json:"items"`
}

type ghItem struct {
	FullName    string `json:"full_name"`
	HTMLURL     string `json:"html_url"`
	Description string `json:"description"`
	Stars       int    `json:"stargazers_count"`
	PushedAt    string `json:"pushed_at"`
	Archived    bool   `json:"archived"`
	Fork        bool   `json:"fork"`
}

// SearchGitHub tìm repository theo topic bằng GitHub best-match.
func (c *Client) SearchGitHub(ctx context.Context, topic string) ([]Repo, error) {
	topic = NormalizeTopic(topic)
	if topic == "" {
		return nil, nil
	}

	endpoint := fmt.Sprintf(
		"https://api.github.com/search/repositories?q=%s&per_page=%d",
		url.QueryEscape(topic), ghPerPage,
	)

	var resp ghResponse
	if err := c.getJSON(ctx, endpoint, &resp); err != nil {
		if strings.Contains(err.Error(), "HTTP 403") || strings.Contains(err.Error(), "HTTP 429") {
			return nil, fmt.Errorf("github: chạm rate limit (10 request/phút khi không có token), thử lại sau một phút")
		}
		return nil, fmt.Errorf("github: %w", err)
	}

	out := make([]Repo, 0, len(resp.Items))
	for _, it := range resp.Items {
		if it.FullName == "" || it.HTMLURL == "" || it.Fork {
			continue
		}
		r := Repo{
			FullName:    it.FullName,
			URL:         it.HTMLURL,
			Description: strings.TrimSpace(it.Description),
			Stars:       it.Stars,
			Archived:    it.Archived,
		}
		if t, err := time.Parse(time.RFC3339, it.PushedAt); err == nil {
			r.Pushed = t.UTC()
		}
		out = append(out, r)
	}
	return out, nil
}
