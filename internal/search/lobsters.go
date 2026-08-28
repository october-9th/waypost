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

const lobstersTagPages = 3

const maxTags = 2

const tagsCacheTTL = 7 * 24 * time.Hour

const lobstersTagsCache = "lobsters-tags.json"

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

	"claude":    "ai",
	"anthropic": "ai",
	"gpt":       "ai",
	"openai":    "ai",
	"copilot":   "ai",
}

// Tag mô tả một tag của lobste.rs.
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

// Tags lấy danh sách tag của lobste.rs, ưu tiên cache trên đĩa.
func (c *Client) Tags(ctx context.Context) ([]Tag, error) {
	if tags, ok := c.readTagsCache(lobstersTagsCache); ok {
		return tags, nil
	}

	var tags []Tag
	if err := c.getJSON(ctx, "https://lobste.rs/tags.json", &tags); err != nil {
		return nil, fmt.Errorf("lobste.rs tags: %w", err)
	}
	c.writeTagsCache(lobstersTagsCache, tags)
	return tags, nil
}

func (c *Client) tagsCachePath(name string) string {
	if c.cacheDir == "" {
		return ""
	}
	return filepath.Join(c.cacheDir, name)
}

func (c *Client) readTagsCache(name string) ([]Tag, bool) {
	path := c.tagsCachePath(name)
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

func (c *Client) writeTagsCache(name string, tags []Tag) {
	path := c.tagsCachePath(name)
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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
	}
}

func matchTags(topic string, tags []Tag, aliases map[string]string) (picked []string, extra []string) {
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
				points += 2 * len(tok)
				consumed = append(consumed, tok)
			case aliases[tok] == t.Tag:
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

// SearchLobsters tìm bài qua tag được suy ra từ topic.
func (c *Client) SearchLobsters(ctx context.Context, topic string) ([]Result, []string, error) {
	topic = NormalizeTopic(topic)
	if topic == "" {
		return nil, nil, nil
	}

	tags, err := c.Tags(ctx)
	if err != nil {
		return nil, nil, err
	}

	picked, extra := matchTags(topic, tags, tagAliases)
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
		if !matchesAny(s.Title+" "+strings.Join(s.Tags, " "), extra) {
			continue
		}
		r := Result{
			Title:       s.Title,
			URL:         s.URL,
			Score:       s.Score,
			Source:      SourceLobsters,
			Type:        TypeVoted,
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

func (c *Client) fetchTagPages(ctx context.Context, tags []string, pages int) ([]lobstersStory, error) {
	joined := strings.Join(tags, ",")
	var all []lobstersStory

	for page := 1; page <= pages; page++ {
		endpoint := fmt.Sprintf("https://lobste.rs/t/%s.json", joined)
		if page > 1 {
			endpoint = fmt.Sprintf("https://lobste.rs/t/%s/page/%d.json", joined, page)
		}

		var stories []lobstersStory
		if err := c.getJSON(ctx, endpoint, &stories); err != nil {
			if len(all) > 0 {
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
