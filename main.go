// albert tìm bài viết kỹ thuật đáng đọc theo topic, xếp hạng bằng điểm
// upvote của Hacker News và lobste.rs thay vì tự chấm điểm bằng heuristic.
//
// Hai frontend dùng chung một lõi internal/search:
//
//	albert                     # TUI — gõ topic, lướt, mở bài, đổi topic
//	albert search "go scheduler"   # một phát ăn ngay, in ra rồi thoát
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
  -n int        số bài hiển thị (TUI 15, search 10)
  -timeout dur  timeout mỗi HTTP request (mặc định 10s)
  -sort string  score (mặc định) | relevance
  -json         in JSON thay vì bảng (chỉ với search)

Topic mới nổi thì thử -sort relevance: điểm cộng đồng chưa kịp hình thành nên
sắp theo điểm là sắp theo nhiễu.

Phím trong TUI:
  enter mở · c thảo luận · y copy link · j/k chọn · tab bài↔repo
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
		// Mọi thứ còn lại là topic gõ thẳng: `albert go scheduler`.
		return runTUI(args)
	}
}

// runTUI mở giao diện tương tác. Chỉ chạy được khi stdout là terminal thật —
// `albert | less` hay chạy trong script thì phải dùng `albert search`.
func runTUI(args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.Usage = usage
	topN := fs.Int("n", 15, "số bài hiển thị")
	timeout := fs.Duration("timeout", 10*time.Second, "timeout mỗi HTTP request")
	sortBy := fs.String("sort", "score", "sắp xếp: score | relevance")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mode, err := parseSort(*sortBy)
	if err != nil {
		return err
	}

	if !isTerminal(os.Stdout) {
		return fmt.Errorf("stdout không phải terminal — dùng `albert search \"<topic>\"` thay vì TUI")
	}

	c := search.NewClient(*timeout, cacheDir())
	return tui.Run(c, strings.Join(fs.Args(), " "), *topN, mode)
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
	timeout := fs.Duration("timeout", 10*time.Second, "timeout mỗi HTTP request")
	asJSON := fs.Bool("json", false, "in JSON thay vì bảng")
	sortBy := fs.String("sort", "score", "sắp xếp: score | relevance")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mode, err := parseSort(*sortBy)
	if err != nil {
		return err
	}

	topic := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(topic) == "" {
		usage()
		return fmt.Errorf("thiếu topic")
	}

	// Ctrl-C hủy request đang chạy thay vì để tiến trình treo tới hết timeout.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	c := search.NewClient(*timeout, cacheDir())
	rep, err := c.Search(ctx, topic, *topN, mode)
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

// parseSort đổi tên mode từ dòng lệnh sang SortMode.
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

// reposShown — CLI in gọn, xem đủ thì mở TUI.
const reposShown = 5

// descMax là độ dài tối đa của mô tả repo in ra CLI.
const descMax = 96

// cacheDir trả về thư mục cache; chuỗi rỗng nếu OS không cho biết, khi đó
// client chạy không cache.
func cacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "albert")
}

func printTable(topic string, rep search.Report) {
	if len(rep.LobstersTags) == 0 {
		fmt.Fprintf(os.Stderr, "lobste.rs: topic %q không khớp tag nào — chỉ dùng Hacker News\n", topic)
	} else {
		fmt.Fprintf(os.Stderr, "lobste.rs: tag %s\n", strings.Join(rep.LobstersTags, ", "))
	}

	if len(rep.Results) == 0 && len(rep.Repos) == 0 {
		fmt.Printf("Không có kết quả cho %q.\n", topic)
		return
	}

	fmt.Println()
	for i, r := range rep.Results {
		year := ""
		if !r.Published.IsZero() {
			year = fmt.Sprintf(" (%d)", r.Published.Year())
		}
		tag := ""
		if r.ShowHN {
			tag = " [Show HN]"
		}
		fmt.Printf("%2d. %5d  %s%s%s  [%s]\n", i+1, r.Score, r.Title, year, tag, r.Source)
		fmt.Printf("           %s\n", r.URL)
	}
	printRepos(rep.Repos)
	fmt.Println()
}

// printRepos in prior art thành mục RIÊNG. Sao GitHub không cùng thang với
// điểm HN nên không trộn chung bảng; và câu hỏi cũng khác — "có ai build
// chưa" chứ không phải "bài nào đáng đọc".
func printRepos(repos []search.Repo) {
	if len(repos) == 0 {
		return
	}
	fmt.Printf("\n── Đã có ai build chưa (GitHub, xếp theo relevance) ──\n\n")
	for i, r := range repos {
		if i >= reposShown {
			break
		}
		note := ""
		if r.Archived {
			note = " [archived]"
		} else if !r.Pushed.IsZero() {
			note = fmt.Sprintf(" [push %d]", r.Pushed.Year())
		}
		fmt.Printf("%2d. %5d★  %s%s\n", i+1, r.Stars, r.FullName, note)
		if d := r.Description; d != "" {
			// Cắt mô tả: README marketing dài cả đoạn, để nguyên thì bảng
			// không lướt được bằng mắt nữa.
			if len([]rune(d)) > descMax {
				d = string([]rune(d)[:descMax-1]) + "…"
			}
			fmt.Printf("            %s\n", d)
		}
	}
}
