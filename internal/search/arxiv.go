package search

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const arxivMaxResults = 10

type arxivEntry struct {
	Title     string `xml:"title"`
	Summary   string `xml:"summary"`
	Published string `xml:"published"`
	Links     []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Type string `xml:"type,attr"`
	} `xml:"link"`
}

type arxivFeed struct {
	Entries []arxivEntry `xml:"entry"`
}

// SearchArXiv tìm paper theo nguyên cụm topic và giữ thứ tự relevance.
func (c *Client) SearchArXiv(ctx context.Context, topic string) ([]Result, error) {
	topic = NormalizeTopic(topic)
	if topic == "" {
		return nil, nil
	}

	endpoint := fmt.Sprintf(
		"https://export.arxiv.org/api/query?search_query=%s&start=0&max_results=%d&sortBy=relevance",
		url.QueryEscape(`all:"`+topic+`"`), arxivMaxResults,
	)

	var feed arxivFeed
	if err := c.getXML(ctx, endpoint, &feed); err != nil {
		return nil, fmt.Errorf("arxiv: %w", err)
	}

	tokens := tokenize(topic)
	out := make([]Result, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		title := squash(e.Title)
		link := e.abs()
		if title == "" || link == "" {
			continue
		}
		if !matchesAll(title, tokens) {
			continue
		}

		r := Result{
			Title:       title,
			URL:         link,
			Score:       0,
			Source:      SourceArXiv,
			Type:        TypeAcademic,
			Description: squash(e.Summary),
		}
		if t, err := time.Parse(time.RFC3339, e.Published); err == nil {
			r.Published = t.UTC()
		}
		out = append(out, r)
	}
	return out, nil
}

func (e arxivEntry) abs() string {
	for _, l := range e.Links {
		if l.Rel == "alternate" && l.Href != "" {
			return l.Href
		}
	}
	for _, l := range e.Links {
		if l.Type != "application/pdf" && l.Href != "" {
			return l.Href
		}
	}
	return ""
}

func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
