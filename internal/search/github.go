package search

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ghPerPage — 15 là đủ cho câu hỏi "có ai build chưa"; xa hơn nữa thì độ liên
// quan tụt nhanh và bạn cũng không đọc hết.
const ghPerPage = 15

// Repo là một repo trên GitHub, dùng để trả lời câu hỏi "đã có ai build thứ
// này chưa" — khác hẳn câu hỏi "bài nào đáng đọc" mà HN/lobste.rs trả lời.
type Repo struct {
	FullName    string    `json:"full_name"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Stars       int       `json:"stars"`
	Pushed      time.Time `json:"pushed"`
	Archived    bool      `json:"archived"`
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

// SearchGitHub tìm repo theo topic.
//
// KHÔNG sort theo stars, dù stars cũng là điểm cộng đồng. Đã đo (2026-08-26):
// `sort=stars` với topic "claude code memory" trả về top 3 là repo 243k/80k/69k
// sao chẳng liên quan gì — cùng một kiểu hỏng như `order=score` của lobste.rs.
// Lý do: stars đo độ nổi tiếng của cả repo, không đo mức liên quan tới query,
// nên topic hẹp mà sort theo stars thì repo to nuốt hết.
//
// Dùng best-match (relevance mặc định của GitHub) và để stars làm ngữ cảnh —
// repo còn sống không, có ai dùng không. Vì vậy Repo nằm ở danh sách RIÊNG,
// không trộn vào bảng xếp hạng của HN/lobste.rs: thang điểm không so được, và
// nó trả lời câu hỏi khác.
//
// Cũng KHÔNG lọc theo token của topic. Đã đo: lọc "khớp ≥2 token" giết
// `rohitg00/agentmemory` (27k sao, "Persistent memory for AI coding agents") —
// prior art thật, trượt chỉ vì tên repo dính liền một token.
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
		// Search API của GitHub cho 10 request/phút khi không kèm token. Gõ
		// nhanh vài topic liên tiếp là chạm ngay, nên nói rõ nguyên nhân thay
		// vì để lại một dòng HTTP 403 khó hiểu.
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
			// Repo archived vẫn giữ: "có ai build chưa" thì một repo đã bỏ
			// hoang vẫn là câu trả lời có. Đánh dấu để bạn tự quyết.
			Archived: it.Archived,
		}
		if t, err := time.Parse(time.RFC3339, it.PushedAt); err == nil {
			r.Pushed = t.UTC()
		}
		out = append(out, r)
	}
	return out, nil
}
