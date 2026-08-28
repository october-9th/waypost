package search

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/net/html"
)

const userAgent = "albert/0.1 (personal article finder CLI; +https://lobste.rs/about)"

const maxBodyBytes = 8 << 20

// Client gọi các nguồn tìm kiếm; khởi tạo bằng NewClient.
type Client struct {
	http     *http.Client
	cacheDir string

	MinScore int

	TrendingLang string
}

// NewClient tạo client với timeout và thư mục cache tùy chọn.
func NewClient(timeout time.Duration, cacheDir string) *Client {
	return &Client{
		http:     &http.Client{Timeout: timeout},
		cacheDir: cacheDir,
		MinScore: hnMinScore,
	}
}

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
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, snippet)
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(dst); err != nil {
		return fmt.Errorf("parse JSON từ %s: %w", url, err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, url, accept string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("tạo request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, snippet)
	}
	return resp.Body, nil
}

func (c *Client) getXML(ctx context.Context, url string, dst any) error {
	body, err := c.get(ctx, url, "application/atom+xml")
	if err != nil {
		return err
	}
	defer body.Close()
	if err := xml.NewDecoder(io.LimitReader(body, maxBodyBytes)).Decode(dst); err != nil {
		return fmt.Errorf("parse XML từ %s: %w", url, err)
	}
	return nil
}

func (c *Client) getHTML(ctx context.Context, url string) (*html.Node, error) {
	body, err := c.get(ctx, url, "text/html")
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := html.Parse(io.LimitReader(body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("parse HTML từ %s: %w", url, err)
	}
	return doc, nil
}
