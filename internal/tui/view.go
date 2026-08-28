package tui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"albert/internal/search"
)

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

const gutter = 9

const sourceWidth = 11

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

	counts := [numPanes]int{len(m.results), len(m.rep.Repos), len(m.rep.Trending)}
	labels := make([]string, 0, numPanes)
	for p := pane(0); p < numPanes; p++ {
		lbl := fmt.Sprintf("%s %d", p, counts[p])
		if p == m.pane {
			lbl = styTab.Render("▸" + lbl)
		}
		labels = append(labels, lbl)
	}
	tabs := strings.Join(labels, styDim.Render(" | "))

	var parts []string
	if n := m.paneLen(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", m.cursor[m.pane]+1, n))
	}
	switch m.pane {
	case paneArticles:
		parts = append(parts, "sắp theo "+m.mode.String())
		parts = append(parts, tagNote("lobste.rs", m.rep.LobstersTags)+" "+tagNote("dev.to", m.rep.DevToTags))
	case paneRepos:
		parts = append(parts, "GitHub search, xếp theo relevance")
	case paneTrending:
		parts = append(parts, "GitHub trending tuần này, không theo topic")
	}
	if len(m.warns) > 0 {
		return " " + styErr.Render(truncate("cảnh báo: "+m.warns[0].Error(), m.width-2))
	}
	rest := styDim.Render(strings.Join(parts, " · "))
	return " " + truncate(tabs+styDim.Render("  ·  ")+rest, m.width-2)
}

func tagNote(source string, tags []string) string {
	if len(tags) == 0 {
		return source + ":—"
	}
	return source + ":" + strings.Join(tags, ",")
}

func (m Model) listView() string {
	n := m.visibleItems()
	lines := make([]string, 0, n*linesPerItem)

	if m.paneLen() == 0 {
		msg := ""
		switch {
		case m.loading:
		case m.err != nil:
			msg = "  không lấy được kết quả — sửa topic rồi thử lại"
		case m.pane == paneTrending:
			msg = "  không lấy được github.com/trending"
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
		if list := m.paneRepoList(); list != nil {
			head, meta = m.repoRow(list[i], sel)
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

func (m Model) articleRow(r search.Result, sel bool) (head, meta string) {
	title := r.Title
	if r.ShowHN {
		title = "[S] " + title
	}
	score := fmt.Sprintf("%5d", r.Score)
	if r.Type == search.TypeAcademic {
		score = "    —"
	}
	head = m.rowHead(score, title, r.Source.String(), sel)

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

func (m Model) repoRow(r search.Repo, sel bool) (head, meta string) {
	right := "—"
	switch {
	case r.StarsPeriod > 0:
		right = fmt.Sprintf("+%s tuần", compactStars(r.StarsPeriod))
	case r.Archived:
		right = "archived"
	case !r.Pushed.IsZero():
		right = fmt.Sprintf("push %d", r.Pushed.Year())
	}
	head = m.rowHead(fmt.Sprintf("%4s★", compactStars(r.Stars)), r.FullName, right, sel)

	desc := strings.Join(strings.Fields(r.Description), " ")
	if r.Language != "" {
		desc = r.Language + " · " + desc
	}
	return head, desc
}

func (m Model) rowHead(left, title, right string, sel bool) string {
	marker := " "
	shown := truncate(title, m.titleWidth())
	if sel {
		marker = stySelected.Render("▸")
		shown = stySelected.Render(shown)
	}
	head := fmt.Sprintf("%s %s  %s", marker, styScore.Render(left), shown)
	if m.width >= narrowWidth {
		pad := m.width - lipgloss.Width(head) - sourceWidth - 1
		if pad < 1 {
			pad = 1
		}
		head += strings.Repeat(" ", pad) + styDim.Render(fmt.Sprintf("%*s", sourceWidth, right))
	}
	return head
}

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
		help = "enter mở · tab đổi pane · s sort · / topic mới · q thoát"
	default:
		help = "enter mở · c thảo luận · y copy · tab bài→repo→trending · s đổi sort · / topic mới · i sửa · r tìm lại · q thoát"
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
