// Package tui cung cấp giao diện terminal cho albert.
package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"albert/internal/search"
)

const (
	defaultWidth  = 80
	defaultHeight = 24

	linesPerItem = 2

	chrome = 8
)

type focus int

const (
	focusInput focus = iota
	focusList
)

type pane int

const (
	paneArticles pane = iota
	paneRepos
	paneTrending
	numPanes
)

// String trả về nhãn ngắn.
func (p pane) String() string {
	switch p {
	case paneRepos:
		return "repo"
	case paneTrending:
		return "trending"
	default:
		return "bài"
	}
}

// Model lưu trạng thái TUI.
type Model struct {
	client *search.Client
	topN   int

	input textinput.Model
	spin  spinner.Model
	focus focus
	pane  pane
	mode  search.SortMode

	seq     int
	loading bool
	cancel  context.CancelFunc

	topic   string
	results []search.Result
	rep     search.Report
	warns   []error
	err     error

	status string

	cursor [numPanes]int
	offset [numPanes]int

	width  int
	height int
}

type startSearchMsg struct{}

type resultsMsg struct {
	seq   int
	topic string
	rep   search.Report
	err   error
}

type actionMsg struct {
	ok  string
	err error
}

// New tạo model TUI và tìm ngay nếu có initialTopic.
func New(c *search.Client, initialTopic string, topN int, mode search.SortMode) Model {
	in := textinput.New()
	in.Placeholder = "topic cần đọc, vd: go scheduler"
	in.Prompt = ""
	in.CharLimit = 120
	in.Width = defaultWidth - 13
	in.SetValue(initialTopic)
	in.CursorEnd()
	in.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		client: c,
		topN:   topN,
		input:  in,
		spin:   sp,
		focus:  focusInput,
		mode:   mode,
		width:  defaultWidth,
		height: defaultHeight,
	}
}

// Run chạy TUI đến khi người dùng thoát.
func Run(c *search.Client, initialTopic string, topN int, mode search.SortMode) error {
	p := tea.NewProgram(New(c, initialTopic, topN, mode), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if strings.TrimSpace(m.input.Value()) != "" {
		cmds = append(cmds, func() tea.Msg { return startSearchMsg{} })
	}
	return tea.Batch(cmds...)
}

func (m Model) startSearch() (Model, tea.Cmd) {
	topic := strings.TrimSpace(m.input.Value())
	if topic == "" {
		return m, nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.seq++
	m.loading = true
	m.err = nil
	m.status = ""

	seq := m.seq
	client := m.client
	topN := m.topN
	mode := m.mode
	run := func() tea.Msg {
		rep, err := client.Search(ctx, topic, topN, mode)
		return resultsMsg{seq: seq, topic: topic, rep: rep, err: err}
	}
	return m, tea.Batch(run, m.spin.Tick)
}

func (m Model) paneLen() int {
	switch m.pane {
	case paneRepos:
		return len(m.rep.Repos)
	case paneTrending:
		return len(m.rep.Trending)
	default:
		return len(m.results)
	}
}

func (m Model) paneRepoList() []search.Repo {
	switch m.pane {
	case paneRepos:
		return m.rep.Repos
	case paneTrending:
		return m.rep.Trending
	}
	return nil
}

func (m Model) selectedResult() (search.Result, bool) {
	i := m.cursor[paneArticles]
	if m.pane != paneArticles || i < 0 || i >= len(m.results) {
		return search.Result{}, false
	}
	return m.results[i], true
}

func (m Model) selectedRepo() (search.Repo, bool) {
	list := m.paneRepoList()
	i := m.cursor[m.pane]
	if list == nil || i < 0 || i >= len(list) {
		return search.Repo{}, false
	}
	return list[i], true
}

func (m Model) currentURL() string {
	if r, ok := m.selectedResult(); ok {
		return r.URL
	}
	if r, ok := m.selectedRepo(); ok {
		return r.URL
	}
	return ""
}

func (m Model) visibleItems() int {
	n := (m.height - chrome) / linesPerItem
	if n < 1 {
		return 1
	}
	return n
}

func (m Model) resize(w, h int) Model {
	m.width, m.height = w, h
	m.input.Width = maxInt(10, w-13)
	return m.clampWindow()
}

func (m Model) clampWindow() Model {
	total := m.paneLen()
	p := m.pane
	if total == 0 {
		m.cursor[p], m.offset[p] = 0, 0
		return m
	}
	if m.cursor[p] < 0 {
		m.cursor[p] = 0
	}
	if m.cursor[p] >= total {
		m.cursor[p] = total - 1
	}
	n := m.visibleItems()
	if m.cursor[p] < m.offset[p] {
		m.offset[p] = m.cursor[p]
	}
	if m.cursor[p] >= m.offset[p]+n {
		m.offset[p] = m.cursor[p] - n + 1
	}
	if m.offset[p] > total-n {
		m.offset[p] = total - n
	}
	if m.offset[p] < 0 {
		m.offset[p] = 0
	}
	return m
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg.Width, msg.Height), nil

	case startSearchMsg:
		return m.startSearch()

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case resultsMsg:
		if msg.seq != m.seq {
			return m, nil // kết quả của lần tìm đã bị thay thế
		}
		m.loading = false
		m.topic = msg.topic
		m.err = msg.err
		m.rep = msg.rep
		m.results = msg.rep.Results
		m.warns = msg.rep.Warnings
		m.cursor = [numPanes]int{}
		m.offset = [numPanes]int{}
		m.pane = paneArticles
		if len(m.results) == 0 && len(m.rep.Repos) > 0 {
			m.pane = paneRepos
		} else if len(m.results) == 0 && len(m.rep.Trending) > 0 {
			m.pane = paneTrending
		}
		if m.paneLen() > 0 {
			m.focus = focusList
			m.input.Blur()
		}
		return m.clampWindow(), nil

	case actionMsg:
		if msg.err != nil {
			m.status = "lỗi: " + msg.err.Error()
		} else {
			m.status = msg.ok
		}
		return m, nil

	case tea.KeyMsg:
		m.status = ""
		if m.focus == focusInput {
			return m.updateInput(msg)
		}
		return m.updateList(msg)
	}

	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.quit()
	case "enter":
		return m.startSearch()
	case "esc":
		if m.paneLen() > 0 {
			m.input.SetValue(m.topic)
			m.input.CursorEnd()
			m.focus = focusList
			m.input.Blur()
			return m, nil
		}
		return m.quit()
	case "tab", "down":
		if m.paneLen() > 0 {
			m.focus = focusList
			m.input.Blur()
			return m, nil
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.pane
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m.quit()

	case "j", "down":
		m.cursor[p]++
		return m.clampWindow(), nil
	case "k", "up":
		m.cursor[p]--
		return m.clampWindow(), nil
	case "g", "home":
		m.cursor[p] = 0
		return m.clampWindow(), nil
	case "G", "end":
		m.cursor[p] = m.paneLen() - 1
		return m.clampWindow(), nil
	case "pgdown", "ctrl+d":
		m.cursor[p] += m.visibleItems()
		return m.clampWindow(), nil
	case "pgup", "ctrl+u":
		m.cursor[p] -= m.visibleItems()
		return m.clampWindow(), nil

	case "tab":
		m.pane = (m.pane + 1) % numPanes
		return m.clampWindow(), nil
	case "shift+tab":
		m.pane = (m.pane + numPanes - 1) % numPanes
		return m.clampWindow(), nil

	case "s":
		if m.mode == search.SortScore {
			m.mode = search.SortRelevance
		} else {
			m.mode = search.SortScore
		}
		m.results = m.rep.Compose(m.mode, m.topN)
		m.cursor[paneArticles], m.offset[paneArticles] = 0, 0
		return m.clampWindow(), nil

	case "/":
		m.focus = focusInput
		m.input.SetValue("")
		return m, m.input.Focus()

	case "i":
		m.focus = focusInput
		m.input.CursorEnd()
		return m, m.input.Focus()

	case "r":
		return m.startSearch()

	case "enter", "o":
		link := m.currentURL()
		if link == "" {
			return m, nil
		}
		return m, openCmd(link, "đã mở trong trình duyệt")

	case "c":
		r, ok := m.selectedResult()
		if !ok {
			m.status = "pane repo không có trang thảo luận"
			return m, nil
		}
		if r.CommentsURL == "" {
			m.status = "bài này không có trang thảo luận"
			return m, nil
		}
		return m, openCmd(r.CommentsURL, "đã mở thảo luận trong trình duyệt")

	case "y":
		link := m.currentURL()
		if link == "" {
			return m, nil
		}
		return m, func() tea.Msg {
			if err := copyClipboard(link); err != nil {
				return actionMsg{err: err}
			}
			return actionMsg{ok: "đã copy link"}
		}
	}
	return m, nil
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	return m, tea.Quit
}

func openCmd(link, ok string) tea.Cmd {
	return func() tea.Msg {
		if err := openURL(link); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{ok: ok}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
