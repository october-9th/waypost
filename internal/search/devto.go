package search

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const devtoTagsCache = "devto-tags.json"

const devtoTagsWanted = 500

const devtoTop = 30

const devtoPerPage = 20

var devtoTagAliases = map[string]string{
	"golang":     "go",
	"k8s":        "kubernetes",
	"postgres":   "database",
	"postgresql": "database",
	"sqlite":     "database",
	"mysql":      "database",
	"sql":        "database",
	"js":         "javascript",
	"ts":         "typescript",
	"llm":        "ai",
	"llms":       "ai",
	"claude":     "ai",
	"anthropic":  "ai",
	"gpt":        "ai",
	"openai":     "ai",
	"copilot":    "ai",
}

type devtoTag struct {
	Name string `json:"name"`
}

type devtoArticle struct {
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	CanonicalURL string   `json:"canonical_url"`
	Reactions    int      `json:"positive_reactions_count"`
	Comments     int      `json:"comments_count"`
	PublishedAt  string   `json:"published_at"`
	Tags         []string `json:"tag_list"`
	Description  string   `json:"description"`
}

func (c *Client) devtoTags(ctx context.Context) ([]Tag, error) {
	if tags, ok := c.readTagsCache(devtoTagsCache); ok {
		return tags, nil
	}

	endpoint := fmt.Sprintf("https://dev.to/api/tags?per_page=%d&page=1", devtoTagsWanted)
	var raw []devtoTag
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("dev.to tags: %w", err)
	}

	tags := make([]Tag, 0, len(raw))
	for _, t := range raw {
		if t.Name == "" {
			continue
		}
		tags = append(tags, Tag{Tag: t.Name, Active: true})
	}
	c.writeTagsCache(devtoTagsCache, tags)
	return tags, nil
}

// SearchDevTo tìm bài dev.to qua tag được suy ra từ topic.
func (c *Client) SearchDevTo(ctx context.Context, topic string) ([]Result, []string, error) {
	topic = NormalizeTopic(topic)
	if topic == "" {
		return nil, nil, nil
	}

	tags, err := c.devtoTags(ctx)
	if err != nil {
		return nil, nil, err
	}

	picked, extra := matchTags(topic, tags, devtoTagAliases)
	if len(picked) == 0 {
		return nil, nil, nil
	}

	var out []Result
	for i, tag := range picked {
		if i > 0 {
			select {
			case <-ctx.Done():
				return out, picked, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}

		endpoint := fmt.Sprintf(
			"https://dev.to/api/articles?tag=%s&top=%d&per_page=%d",
			url.QueryEscape(tag), devtoTop, devtoPerPage,
		)
		var arts []devtoArticle
		if err := c.getJSON(ctx, endpoint, &arts); err != nil {
			if len(out) > 0 {
				return out, picked, nil // đã có tag trước, dùng tạm
			}
			return nil, picked, fmt.Errorf("dev.to tag %q: %w", tag, err)
		}

		for _, a := range arts {
			r, ok := a.result(extra)
			if ok {
				out = append(out, r)
			}
		}
	}
	return out, picked, nil
}

func (a devtoArticle) result(extra []string) (Result, bool) {
	link := a.URL
	if link == "" || a.Title == "" {
		return Result{}, false
	}
	if cu := a.CanonicalURL; cu != "" && !strings.Contains(cu, "dev.to/") {
		link = cu
	}
	if !matchesAny(a.Title+" "+strings.Join(a.Tags, " "), extra) {
		return Result{}, false
	}

	r := Result{
		Title:       a.Title,
		URL:         link,
		Score:       a.Reactions,
		Source:      SourceDevTo,
		Type:        TypeBlog,
		NumComments: a.Comments,
		Description: strings.TrimSpace(a.Description),
	}
	if a.URL != "" {
		r.CommentsURL = a.URL + "#comments"
	}
	if t, err := time.Parse(time.RFC3339, a.PublishedAt); err == nil {
		r.Published = t.UTC()
	}
	return r, true
}
