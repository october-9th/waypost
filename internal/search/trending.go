package search

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const trendingSince = "weekly"

// SearchTrending lấy GitHub Trending theo ngôn ngữ và khoảng thời gian.
func (c *Client) SearchTrending(ctx context.Context, lang string) ([]Repo, error) {
	endpoint := "https://github.com/trending"
	if lang != "" {
		endpoint += "/" + url.PathEscape(lang)
	}
	endpoint += "?since=" + trendingSince

	doc, err := c.getHTML(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("github trending: %w", err)
	}

	rows := findAll(doc, func(n *html.Node) bool {
		return n.Data == "article" && strings.Contains(attr(n, "class"), "Box-row")
	})
	if len(rows) == 0 {
		return nil, fmt.Errorf("github trending: không parse được dòng nào — nhiều khả năng GitHub đổi HTML")
	}

	out := make([]Repo, 0, len(rows))
	for _, row := range rows {
		if r, ok := parseTrendingRow(row); ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func parseTrendingRow(row *html.Node) (Repo, bool) {
	link := find(row, func(n *html.Node) bool {
		return n.Data == "a" && n.Parent != nil && n.Parent.Data == "h2"
	})
	if link == nil {
		return Repo{}, false
	}
	name := strings.Trim(attr(link, "href"), "/")
	if name == "" || strings.Count(name, "/") != 1 {
		return Repo{}, false
	}

	r := Repo{
		FullName: name,
		URL:      "https://github.com/" + name,
		Period:   trendingSince,
	}

	if p := find(row, func(n *html.Node) bool { return n.Data == "p" }); p != nil {
		r.Description = squash(text(p))
	}
	if l := find(row, func(n *html.Node) bool {
		return n.Data == "span" && attr(n, "itemprop") == "programmingLanguage"
	}); l != nil {
		r.Language = squash(text(l))
	}
	if s := find(row, func(n *html.Node) bool {
		return n.Data == "a" && strings.HasSuffix(attr(n, "href"), "/stargazers")
	}); s != nil {
		r.Stars = parseCount(text(s))
	}
	if d := find(row, func(n *html.Node) bool {
		return n.Data == "span" && strings.Contains(attr(n, "class"), "float-sm-right")
	}); d != nil {
		r.StarsPeriod = parseCount(text(d))
	}
	return r, true
}

func parseCount(s string) int {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ',':
		default:
			if b.Len() > 0 {
				goto done
			}
		}
	}
done:
	n, err := strconv.Atoi(b.String())
	if err != nil {
		return 0
	}
	return n
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func find(root *html.Node, ok func(*html.Node) bool) *html.Node {
	if root.Type == html.ElementNode && ok(root) {
		return root
	}
	for ch := root.FirstChild; ch != nil; ch = ch.NextSibling {
		if got := find(ch, ok); got != nil {
			return got
		}
	}
	return nil
}

func findAll(root *html.Node, ok func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && ok(n) {
			out = append(out, n)
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return out
}

func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return b.String()
}
