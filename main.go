package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"albert/internal/search"
	"albert/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "albert:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `albert — tìm bài kỹ thuật đáng đọc theo topic

  albert [flags] ["<topic>"]        mở TUI (kèm topic thì tìm luôn)
  albert search [flags] "<topic>"   in kết quả ra stdout rồi thoát

Flags:
  -n int         số bài hiển thị (TUI 15, search 10)
  -timeout dur   timeout mỗi HTTP request (mặc định 10s)
  -sort string   score (mặc định) | relevance
  -min-score int sàn điểm cho bài Hacker News (mặc định 10, 0 = không cắt)
  -lang string   ngôn ngữ cho GitHub trending (mặc định đoán từ topic)
  -json          in JSON thay vì bảng (chỉ với search)

Topic mới nổi thì thử -sort relevance: điểm cộng đồng chưa kịp hình thành nên
sắp theo điểm là sắp theo nhiễu. Ở mode đó -min-score cũng tự tắt.

Nguồn: Hacker News + lobste.rs xếp chung một bảng; dev.to và arXiv là phụ lục
ở cuối vì thang điểm không so được (arXiv không có điểm nào, in "—").

Phím trong TUI:
  enter mở · c thảo luận · y copy link · j/k chọn · tab bài→repo→trending
  s đổi sort · / topic mới · i sửa topic · r tìm lại · q thoát
`)
}

func run(args []string) error {
	switch {
	case len(args) == 0:
		return runTUI(nil)
	case args[0] == "help", args[0] == "-h", args[0] == "--help":
		usage()
		return nil
	case args[0] == "search":
		return runSearch(args[1:])
	case args[0] == "tui":
		return runTUI(args[1:])
	default:
		return runTUI(args)
	}
}

func runTUI(args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.Usage = usage
	topN := fs.Int("n", 15, "số bài hiển thị")
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	mode, err := parseSort(*cf.sortBy)
	if err != nil {
		return err
	}

	if !isTerminal(os.Stdout) {
		return fmt.Errorf("stdout không phải terminal — dùng `albert search \"<topic>\"` thay vì TUI")
	}

	return tui.Run(cf.client(), strings.Join(fs.Args(), " "), *topN, mode)
}

type commonFlags struct {
	timeout  *time.Duration
	sortBy   *string
	minScore *int
	lang     *string
}

func addCommonFlags(fs *flag.FlagSet) commonFlags {
	return commonFlags{
		timeout:  fs.Duration("timeout", 10*time.Second, "timeout mỗi HTTP request"),
		sortBy:   fs.String("sort", "score", "sắp xếp: score | relevance"),
		minScore: fs.Int("min-score", 10, "sàn điểm cho bài Hacker News (0 = không cắt)"),
		lang:     fs.String("lang", "", "ngôn ngữ cho GitHub trending (rỗng = đoán từ topic)"),
	}
}

func (cf commonFlags) client() *search.Client {
	c := search.NewClient(*cf.timeout, cacheDir())
	c.MinScore = *cf.minScore
	c.TrendingLang = *cf.lang
	return c
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func runSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.Usage = usage
	topN := fs.Int("n", 10, "số bài hiển thị")
	asJSON := fs.Bool("json", false, "in JSON thay vì bảng")
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	mode, err := parseSort(*cf.sortBy)
	if err != nil {
		return err
	}

	topic := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(topic) == "" {
		usage()
		return fmt.Errorf("thiếu topic")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	rep, err := cf.client().Search(ctx, topic, *topN, mode)
	if err != nil {
		return err
	}

	for _, w := range rep.Warnings {
		fmt.Fprintln(os.Stderr, "cảnh báo:", w)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep.Results)
	}

	printTable(topic, rep)
	return nil
}

func parseSort(s string) (search.SortMode, error) {
	switch s {
	case "score", "":
		return search.SortScore, nil
	case "relevance", "rel":
		return search.SortRelevance, nil
	default:
		return 0, fmt.Errorf("-sort không hợp lệ: %q (score hoặc relevance)", s)
	}
}

const reposShown = 5

const descMax = 96

func cacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "albert")
}

func printTable(topic string, rep search.Report) {
	printTagNote("lobste.rs", topic, rep.LobstersTags)
	printTagNote("dev.to", topic, rep.DevToTags)

	if len(rep.Results) == 0 && len(rep.Repos) == 0 && len(rep.Trending) == 0 {
		fmt.Printf("Không có kết quả cho %q.\n", topic)
		return
	}

	fmt.Println()
	last := search.ContentType(255)
	for i, r := range rep.Results {
		if r.Type != last {
			fmt.Print(groupHeading(r.Type))
			last = r.Type
		}
		year := ""
		if !r.Published.IsZero() {
			year = fmt.Sprintf(" (%d)", r.Published.Year())
		}
		tag := ""
		if r.ShowHN {
			tag = " [Show HN]"
		}
		score := fmt.Sprintf("%5d", r.Score)
		if r.Type == search.TypeAcademic {
			score = "    —"
		}
		fmt.Printf("%2d. %s  %s%s%s  [%s]\n", i+1, score, r.Title, year, tag, r.Source)
		fmt.Printf("           %s\n", r.URL)
	}
	printRepos("Đã có ai build chưa (GitHub search, xếp theo relevance)", rep.Repos)
	printRepos("Tuần này nổi gì (GitHub trending, KHÔNG theo topic)", rep.Trending)
	fmt.Println()
}

func printTagNote(source, topic string, tags []string) {
	if len(tags) == 0 {
		fmt.Fprintf(os.Stderr, "%s: topic %q không khớp tag nào — bỏ qua nguồn này\n", source, topic)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: tag %s\n", source, strings.Join(tags, ", "))
}

func groupHeading(t search.ContentType) string {
	switch t {
	case search.TypeBlog:
		return "\n── dev.to (reaction, thang điểm thấp hơn HN hai bậc) ──\n\n"
	case search.TypeAcademic:
		return "\n── arXiv (preprint — CHƯA có tín hiệu chất lượng nào) ──\n\n"
	default:
		return ""
	}
}

func printRepos(heading string, repos []search.Repo) {
	if len(repos) == 0 {
		return
	}
	fmt.Printf("\n── %s ──\n\n", heading)
	for i, r := range repos {
		if i >= reposShown {
			break
		}
		note := ""
		switch {
		case r.StarsPeriod > 0:
			note = fmt.Sprintf(" [+%d tuần này]", r.StarsPeriod)
		case r.Archived:
			note = " [archived]"
		case !r.Pushed.IsZero():
			note = fmt.Sprintf(" [push %d]", r.Pushed.Year())
		}
		fmt.Printf("%2d. %5d★  %s%s\n", i+1, r.Stars, r.FullName, note)
		if d := r.Description; d != "" {
			if len([]rune(d)) > descMax {
				d = string([]rune(d)[:descMax-1]) + "…"
			}
			fmt.Printf("            %s\n", d)
		}
	}
}
