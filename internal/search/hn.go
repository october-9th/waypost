package search

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const hnHitsPerPage = 30

type hnResponse struct {
	Hits []hnHit `json:"hits"`
}

type hnHit struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Points      int      `json:"points"`
	CreatedAtI  int64    `json:"created_at_i"`
	ObjectID    string   `json:"objectID"`
	NumComments int      `json:"num_comments"`
	Tags        []string `json:"_tags"`
}

func (h hnHit) showHN() bool {
	for _, t := range h.Tags {
		if t == "show_hn" {
			return true
		}
	}
	return false
}

// SearchHN tìm bài Hacker News qua Algolia.
func (c *Client) SearchHN(ctx context.Context, topic string) ([]Result, error) {
	topic = NormalizeTopic(topic)
	if topic == "" {
		return nil, nil
	}

	endpoint := fmt.Sprintf(
		"https://hn.algolia.com/api/v1/search?query=%s&tags=story&hitsPerPage=%d",
		url.QueryEscape(topic), hnHitsPerPage,
	)

	var resp hnResponse
	if err := c.getJSON(ctx, endpoint, &resp); err != nil {
		return nil, fmt.Errorf("hacker news: %w", err)
	}

	tokens := tokenize(topic)
	out := make([]Result, 0, len(resp.Hits))
	for _, h := range resp.Hits {
		if h.URL == "" || h.Title == "" {
			continue
		}
		if !matchesAny(h.Title+" "+h.URL, tokens) {
			continue
		}
		r := Result{
			Title:       h.Title,
			URL:         h.URL,
			Score:       h.Points,
			Source:      SourceHN,
			Type:        TypeVoted,
			NumComments: h.NumComments,
			ShowHN:      h.showHN(),
		}
		if h.ObjectID != "" {
			r.CommentsURL = "https://news.ycombinator.com/item?id=" + h.ObjectID
		}
		if h.CreatedAtI > 0 {
			r.Published = time.Unix(h.CreatedAtI, 0).UTC()
		}
		out = append(out, r)
	}
	return out, nil
}
