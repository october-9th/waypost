package tui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"albert/internal/search"
)

// Dùng màu ANSI cơ bản (0-15) thay vì mã hex: chúng lấy theo bảng màu người
// dùng đã chỉnh cho terminal của mình, nên hợp cả nền sáng lẫn nền tối.
var (
	styBrand    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styScore    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	stySelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styLink     = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styTab      = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
)

// gutter là bề rộng cột trái của dòng tiêu đề: marker(1) + " " + điểm(5) +
// "  ". Dòng phụ thụt vào đúng bằng đó để hai cột thẳng hàng.
const gutter = 9

// sourceWidth đủ cho nhãn dài nhất, "HN+Lobsters".
const sourceWidth = 11

// narrowWidth là ngưỡng bỏ cột nguồn bên phải. Hẹp hơn mức này thì 11 ô dành
// cho cột đó ăn mất quá nhiều tiêu đề, nên nguồn chuyển xuống dòng phụ.
const narrowWidth = 72

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteByte('\n')
	b.WriteString(m.metaView())
	b.WriteString("\n\n")
	b.WriteString(m.listView())
	b.WriteByte('\n')
	b.WriteString(m.detailView())
	b.WriteByte('\n')
	b.WriteString(m.footerView())
	return b.String()
}

func (m Model) headerView() string {
	prompt := styDim.Render("  ")
	if m.focus == focusInput {
		prompt = styBrand.Render("▸ ")
	}
	return " " + styBrand.Render("albert") + " " + prompt + m.input.View()
}

// metaView là dòng cho biết chuyện gì đang xảy ra: đang xem pane nào, sắp xếp
// kiểu gì, tìm được bao nhiêu, lobste.rs có góp gì không, nguồn nào hỏng.
func (m Model) metaView() string {
	switch {
	case m.loading:
		return " " + m.spin.View() + styDim.Render(" đang tìm…")
	case m.status != "":
		if strings.HasPrefix(m.status, "lỗi") {
			return " " + styErr.Render(truncate(m.status, m.width-2))
		}
		return " " + styOK.Render(truncate(m.status, m.width-2))
	case m.err != nil:
		return " " + styErr.Render(truncate("lỗi: "+m.err.Error(), m.width-2))
	case m.topic == "":
		return " " + styDim.Render("gõ topic rồi Enter")
	}

	// Nhãn pane luôn hiện cả hai để biết bên kia có gì mà bấm tab.
	tabs := fmt.Sprintf("bài %d | repo %d", len(m.results), len(m.repos))
	if m.pane == paneRepos {
		tabs = fmt.Sprintf("bài %d | ", len(m.results)) + styTab.Render(fmt.Sprintf("▸repo %d", len(m.repos)))
	} else {
		tabs = styTab.Render(fmt.Sprintf("▸bài %d", len(m.results))) + fmt.Sprintf(" | repo %d", len(m.repos))
	}

	var parts []string
	if n := m.paneLen(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", m.cursor[m.pane]+1, n))
	}
	if m.pane == paneArticles {
		parts = append(parts, "sắp theo "+m.mode.String())
		if len(m.tags) == 0 {
			parts = append(parts, "lobste.rs: không khớp tag")
		} else {
			parts = append(parts, "lobste.rs: "+strings.Join(m.tags, ", "))
		}
	} else {
		parts = append(parts, "GitHub, xếp theo relevance")
	}
	if len(m.warns) > 0 {
		return " " + styErr.Render(truncate("cảnh báo: "+m.warns[0].Error(), m.width-2))
	}
	rest := styDim.Render(strings.Join(parts, " · "))
	return " " + truncate(tabs+styDim.Render("  ·  ")+rest, m.width-2)
}

// listView vẽ đúng visibleItems*linesPerItem dòng, đệm dòng trống cho đủ, để
// khung chi tiết và footer luôn nằm yên một chỗ khi số kết quả thay đổi.
func (m Model) listView() string {
	n := m.visibleItems()
	lines := make([]string, 0, n*linesPerItem)

	if m.paneLen() == 0 {
		msg := ""
		switch {
		case m.loading:
		case m.err != nil:
			msg = "  không lấy được kết quả — sửa topic rồi thử lại"
		case m.pane == paneRepos && m.topic != "":
			msg = fmt.Sprintf("  không có repo nào cho %q", m.topic)
		case m.topic != "":
			msg = fmt.Sprintf("  không có bài nào cho %q", m.topic)
		}
		lines = append(lines, styDim.Render(msg))
	}

	off := m.offset[m.pane]
	for i := off; i < m.paneLen() && len(lines) < n*linesPerItem; i++ {
		sel := i == m.cursor[m.pane]
		var head, meta string
		if m.pane == paneRepos {
			head, meta = m.repoRow(m.repos[i], sel)
		} else {
			head, meta = m.articleRow(m.results[i], sel)
		}
		lines = append(lines, head)
		lines = append(lines, strings.Repeat(" ", gutter)+styDim.Render(truncate(meta, m.width-gutter-1)))
	}

	for len(lines) < n*linesPerItem {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// articleRow vẽ một bài: điểm cộng đồng + tiêu đề + nguồn.
func (m Model) articleRow(r search.Result, sel bool) (head, meta string) {
	title := r.Title
	if r.ShowHN {
		// Không lọc Show HN — với câu hỏi "có ai build chưa" thì nó chính là
		// câu trả lời. Chỉ đánh dấu để phân biệt sản phẩm với bài viết.
		title = "[S] " + title
	}
	head = m.rowHead(fmt.Sprintf("%5d", r.Score), title, r.Source.String(), sel)

	var parts []string
	if m.width < narrowWidth {
		parts = append(parts, r.Source.String())
	}
	parts = append(parts, domainOf(r.URL))
	if !r.Published.IsZero() {
		parts = append(parts, fmt.Sprintf("%d", r.Published.Year()))
	}
	if r.NumComments > 0 {
		parts = append(parts, fmt.Sprintf("%d thảo luận", r.NumComments))
	}
	return head, strings.Join(parts, " · ")
}

// repoRow vẽ một repo: sao + tên + trạng thái. Sao ở đây là ngữ cảnh (có ai
// dùng không), KHÔNG phải thứ hạng — danh sách này xếp theo relevance.
func (m Model) repoRow(r search.Repo, sel bool) (head, meta string) {
	// Cột phải nói repo còn sống hay không — đó mới là thứ cần biết khi hỏi
	// "có ai build chưa". Nhãn "GitHub" thì mọi dòng đều giống nhau, vô dụng.
	right := "—"
	if r.Archived {
		right = "archived"
	} else if !r.Pushed.IsZero() {
		right = fmt.Sprintf("push %d", r.Pushed.Year())
	}
	head = m.rowHead(fmt.Sprintf("%4s★", compactStars(r.Stars)), r.FullName, right, sel)

	// Số sao đã nằm ở cột trái, đừng in lại lần nữa ở dòng phụ.
	return head, strings.Join(strings.Fields(r.Description), " ")
}

// rowHead dựng dòng tiêu đề chung cho cả hai pane.
func (m Model) rowHead(left, title, right string, sel bool) string {
	marker := " "
	shown := truncate(title, m.titleWidth())
	if sel {
		marker = stySelected.Render("▸")
		shown = stySelected.Render(shown)
	}
	head := fmt.Sprintf("%s %s  %s", marker, styScore.Render(left), shown)
	if m.width >= narrowWidth {
		// Đệm theo bề rộng thật của phần đã vẽ, không theo titleWidth: tiêu đề
		// ngắn hơn cột thì nhãn bên phải vẫn phải nằm sát mép phải.
		pad := m.width - lipgloss.Width(head) - sourceWidth - 1
		if pad < 1 {
			pad = 1
		}
		head += strings.Repeat(" ", pad) + styDim.Render(fmt.Sprintf("%*s", sourceWidth, right))
	}
	return head
}

// compactStars rút gọn số sao cho vừa cột 5 ô: 243302 → 243k.
func compactStars(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%dM", n/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// detailView là khung 2 dòng cho mục đang chọn: link đầy đủ và mô tả.
func (m Model) detailView() string {
	link, desc := "", ""
	if r, ok := m.selectedResult(); ok {
		link = r.URL
		desc = r.Description
	} else if r, ok := m.selectedRepo(); ok {
		link = r.URL
		desc = r.Description
	} else {
		return "\n"
	}

	out := " " + styLink.Render(truncate(link, m.width-2))
	if d := strings.Join(strings.Fields(desc), " "); d != "" {
		out += "\n " + styDim.Render(truncate(d, m.width-2))
	} else {
		out += "\n"
	}
	return out
}

func (m Model) footerView() string {
	var help string
	switch {
	case m.focus == focusInput:
		help = "enter tìm · esc danh sách · ctrl+c thoát"
	case m.width < narrowWidth:
		help = "enter mở · tab bài↔repo · s sort · / topic mới · q thoát"
	default:
		help = "enter mở · c thảo luận · y copy · tab bài↔repo · s đổi sort · / topic mới · i sửa · r tìm lại · q thoát"
	}
	return " " + styDim.Render(truncate(help, m.width-2))
}

func (m Model) titleWidth() int {
	w := m.width - gutter - 2
	if m.width >= narrowWidth {
		w -= sourceWidth
	}
	if w < 20 {
		return 20
	}
	return w
}

func domainOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

// truncate cắt chuỗi theo bề rộng hiển thị (không phải số byte hay số rune —
// tiêu đề hay có ký tự CJK rộng gấp đôi).
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}
