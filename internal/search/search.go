package search

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const appendixMax = 5

// Report chứa kết quả và trạng thái của từng nguồn.
type Report struct {
	Results []Result

	Merged []Result

	Blog   []Result
	Papers []Result

	Repos []Repo

	Trending []Repo

	LobstersTags []string
	DevToTags    []string

	Warnings []error
}

// Compose lọc, sắp xếp và nối các nhóm kết quả.
func (r Report) Compose(mode SortMode, topN int) []Result {
	voted := SortResults(r.Merged, mode)
	if topN > 0 && len(voted) > topN {
		voted = voted[:topN]
	}

	blog := SortResults(r.Blog, SortScore)
	if len(blog) > appendixMax {
		blog = blog[:appendixMax]
	}

	papers := r.Papers
	if len(papers) > appendixMax {
		papers = papers[:appendixMax]
	}

	out := make([]Result, 0, len(voted)+len(blog)+len(papers))
	out = append(out, voted...)
	out = append(out, blog...)
	out = append(out, papers...)
	return out
}

// Search truy vấn các nguồn và giữ lại kết quả một phần khi một nguồn lỗi.
func (c *Client) Search(ctx context.Context, topic string, topN int, mode SortMode) (Report, error) {
	topic = NormalizeTopic(topic)
	if topic == "" {
		return Report{}, errors.New("topic rỗng")
	}

	var (
		wg       sync.WaitGroup
		hnHits   []Result
		hnErr    error
		lobHits  []Result
		lobTags  []string
		lobErr   error
		devHits  []Result
		devTags  []string
		devErr   error
		papers   []Result
		arxErr   error
		repos    []Repo
		ghErr    error
		trending []Repo
		trendErr error
	)

	wg.Add(6)
	go func() {
		defer wg.Done()
		hnHits, hnErr = c.SearchHN(ctx, topic)
	}()
	go func() {
		defer wg.Done()
		lobHits, lobTags, lobErr = c.SearchLobsters(ctx, topic)
	}()
	go func() {
		defer wg.Done()
		devHits, devTags, devErr = c.SearchDevTo(ctx, topic)
	}()
	go func() {
		defer wg.Done()
		papers, arxErr = c.SearchArXiv(ctx, topic)
	}()
	go func() {
		defer wg.Done()
		repos, ghErr = c.SearchGitHub(ctx, topic)
	}()
	go func() {
		defer wg.Done()
		lang := c.TrendingLang
		if lang == "" {
			lang = trendingLangOf(topic)
		}
		trending, trendErr = c.SearchTrending(ctx, lang)
	}()
	wg.Wait()

	if hnErr != nil && lobErr != nil {
		return Report{}, fmt.Errorf("cả hai nguồn chính đều hỏng: %w", errors.Join(hnErr, lobErr))
	}

	if mode == SortScore {
		hnHits = applyMinScore(hnHits, c.MinScore, hnMinKeep)
	}

	rep := Report{
		LobstersTags: lobTags,
		DevToTags:    devTags,
		Repos:        repos,
		Trending:     trending,
		Blog:         devHits,
		Papers:       papers,
	}
	for _, err := range []error{hnErr, lobErr, devErr, arxErr, ghErr, trendErr} {
		if err != nil {
			rep.Warnings = append(rep.Warnings, err)
		}
	}

	rep.Merged = Merge(hnHits, lobHits)
	rep.Blog = dropSeen(rep.Blog, rep.Merged)
	rep.Papers = dropSeen(rep.Papers, rep.Merged)
	rep.Results = rep.Compose(mode, topN)
	return rep, nil
}

var trendingLangs = map[string]string{
	"go": "go", "golang": "go", "rust": "rust", "python": "python", "py": "python",
	"javascript": "javascript", "js": "javascript", "typescript": "typescript", "ts": "typescript",
	"java": "java", "kotlin": "kotlin", "swift": "swift", "ruby": "ruby", "php": "php",
	"elixir": "elixir", "erlang": "erlang", "haskell": "haskell", "zig": "zig", "lua": "lua",
	"c": "c", "c++": "c++", "cpp": "c++", "c#": "c#", "csharp": "c#", "scala": "scala",
	"clojure": "clojure", "ocaml": "ocaml", "dart": "dart", "julia": "julia",
	"shell": "shell", "bash": "shell", "nix": "nix",
}

func trendingLangOf(topic string) string {
	for _, tok := range tokenize(topic) {
		if lang, ok := trendingLangs[tok]; ok {
			return lang
		}
	}
	return ""
}
