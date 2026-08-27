package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// userAgent tự giới thiệu rõ ràng. lobste.rs là site nhỏ tự host, để UA mặc
// định của Go hoặc giả làm trình duyệt đều là hành vi xấu.
const userAgent = "albert/0.1 (personal article finder CLI; +https://lobste.rs/about)"

// maxBodyBytes chặn response quá lớn. Hai API này trả vài chục KB; nếu có gì
// trả về hàng chục MB thì đó là dấu hiệu hỏng, không phải dữ liệu cần đọc.
const maxBodyBytes = 8 << 20

// Client gọi cả hai nguồn. Zero value không dùng được — tạo bằng NewClient.
type Client struct {
	http     *http.Client
	cacheDir string
}

// NewClient tạo client với timeout cho mỗi request. cacheDir dùng để cache
// danh sách tag của lobste.rs; để rỗng thì không cache.
func NewClient(timeout time.Duration, cacheDir string) *Client {
	return &Client{
		http:     &http.Client{Timeout: timeout},
		cacheDir: cacheDir,
	}
}

// getJSON gọi GET và decode JSON vào dst.
func (c *Client) getJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("tạo request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Đọc một ít body vào thông báo lỗi: lobste.rs trả lý do dạng JSON
		// (vd 400 "Unpermitted query or form parameter"), rất đáng để thấy.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, snippet)
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(dst); err != nil {
		return fmt.Errorf("parse JSON từ %s: %w", url, err)
	}
	return nil
}
