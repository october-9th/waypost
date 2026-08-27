// Package tui là frontend thứ hai của albert, bên cạnh CLI một phát ăn ngay.
//
// Lý do tồn tại: vòng lặp dùng thật là "gõ topic → lướt kết quả → mở vài bài →
// đổi topic thử lại". Chạy lại lệnh cho mỗi vòng thì mệt.
//
// Toàn bộ logic tìm kiếm nằm ở internal/search; package này chỉ vẽ và bắt phím.
package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"albert/internal/search"
)

// Kích thước giả định trước khi terminal báo kích thước thật.
const (
	defaultWidth  = 80
	defaultHeight = 24

	// linesPerItem là số dòng mỗi kết quả chiếm trong danh sách. Cố định để
	// phép tính cửa sổ cuộn không phụ thuộc nội dung từng bài.
	linesPerItem = 2

	// chrome là số dòng dành cho header, khoảng trắng, khung chi tiết và
	// footer — phần còn lại của màn hình mới là danh sách.
	chrome = 8
)

type focus int

const (
	focusInput focus = iota
	focusList
)

// pane chọn danh sách đang xem. Hai danh sách trả lời hai câu hỏi khác nhau
// nên không trộn vào nhau: "bài nào đáng đọc" và "đã có ai build chưa".
type pane int

const (
	paneArticles pane = iota
	paneRepos
	numPanes
)

// Model là state của TUI. Tạo bằng New.
type Model struct {
	client *search.Client
	topN   int

	input textinput.Model
	spin  spinner.Model
	focus focus
	pane  pane
	mode  search.SortMode

	// seq tăng mỗi lần bắt đầu tìm. Kết quả về trễ mà seq không khớp thì bỏ:
	// gõ topic mới trong lúc topic cũ chưa xong là chuyện bình thường.
	seq     int
	loading bool
	cancel  context.CancelFunc

	topic   string
	results []search.Result
	// merged là toàn bộ kết quả theo thứ tự relevance gốc; giữ lại để đổi
	// sort tại chỗ, không gọi lại API.
	merged []search.Result
	repos  []search.Repo
	tags   []string
	warns  []error
	err    error

	// status là thông báo tạm cho hành động vừa làm (mở link, copy). Xoá ngay
	// khi bấm phím tiếp theo để không đọng lại thông tin cũ.
	status string

	// Con trỏ và cửa sổ cuộn riêng cho từng pane — chuyển qua chuyển lại mà
	// mất chỗ đang đọc thì rất khó chịu.
	cursor [numPanes]int
	offset [numPanes]int

	width  int
	height int
}

// startSearchMsg khởi động lần tìm đầu tiên khi mở TUI kèm sẵn topic.
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

// New tạo model. initialTopic khác rỗng thì tìm luôn khi khởi động.
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

// Run chạy TUI cho tới khi người dùng thoát.
func Run(c *search.Client, initialTopic string, topN int, mode search.SortMode) error {
	p := tea.NewProgram(New(c, initialTopic, topN, mode), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if strings.TrimSpace(m.input.Value()) != "" {
		// Init chỉ trả về Cmd, không sửa được model, nên phải đi vòng qua một
		// message thì seq/loading mới thật sự vào state.
		cmds = append(cmds, func() tea.Msg { return startSearchMsg{} })
	}
	return tea.Batch(cmds...)
}

// startSearch huỷ lần tìm đang chạy (nếu có) rồi bắt đầu lần mới.
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

// paneLen là số mục của pane đang xem.
func (m Model) paneLen() int {
	if m.pane == paneRepos {
		return len(m.repos)
	}
	return len(m.results)
}

func (m Model) selectedResult() (search.Result, bool) {
	i := m.cursor[paneArticles]
	if m.pane != paneArticles || i < 0 || i >= len(m.results) {
		return search.Result{}, false
	}
	return m.results[i], true
}

func (m Model) selectedRepo() (search.Repo, bool) {
	i := m.cursor[paneRepos]
	if m.pane != paneRepos || i < 0 || i >= len(m.repos) {
		return search.Repo{}, false
	}
	return m.repos[i], true
}

// currentURL là link của mục đang chọn, bất kể đang ở pane nào.
func (m Model) currentURL() string {
	if r, ok := m.selectedResult(); ok {
		return r.URL
	}
	if r, ok := m.selectedRepo(); ok {
		return r.URL
	}
	return ""
}

// visibleItems là số mục vẽ vừa màn hình hiện tại.
func (m Model) visibleItems() int {
	n := (m.height - chrome) / linesPerItem
	if n < 1 {
		return 1
	}
	return n
}

// resize áp kích thước terminal mới. Ô nhập có Width riêng nên phải chỉnh
// cùng lúc, nếu không textinput vẫn đệm khoảng trắng theo bề rộng cũ và làm
// dòng header tràn ra ngoài.
func (m Model) resize(w, h int) Model {
	m.width, m.height = w, h
	// headerView đã tiêu " albert " + marker; trừ dư một ô cho con trỏ mà
	// textinput vẽ thêm ở cuối.
	m.input.Width = maxInt(10, w-13)
	return m.clampWindow()
}

// clampWindow kéo cửa sổ cuộn về đúng vị trí con trỏ của pane đang xem.
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
		m.results = msg.rep.Results
		m.merged = msg.rep.Merged
		m.repos = msg.rep.Repos
		m.tags = msg.rep.LobstersTags
		m.warns = msg.rep.Warnings
		m.cursor = [numPanes]int{}
		m.offset = [numPanes]int{}
		m.pane = paneArticles
		// Topic mới nổi hay ra 0 bài mà vẫn có repo. Nhảy thẳng sang pane repo
		// thay vì bắt người dùng đoán là còn thứ khác đang nấp sau phím tab.
		if len(m.results) == 0 && len(m.repos) > 0 {
			m.pane = paneRepos
		}
		if m.paneLen() > 0 {
			// Có kết quả thì chuyển focus sang danh sách luôn, để j/k dùng
			// được ngay mà không phải bấm thêm phím nào.
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
			// Trả ô nhập về topic của kết quả đang xem: bỏ dở việc gõ mà để
			// lại chữ nửa vời trên header thì header nói dối về danh sách.
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
		if m.pane == paneArticles {
			m.pane = paneRepos
		} else {
			m.pane = paneArticles
		}
		return m.clampWindow(), nil

	case "s":
		// Đổi sort tại chỗ từ m.merged — không gọi lại API. lobste.rs là site
		// nhỏ tự host, đừng bắt nó phục vụ lại chỉ vì đổi cách sắp xếp.
		if m.mode == search.SortScore {
			m.mode = search.SortRelevance
		} else {
			m.mode = search.SortScore
		}
		m.results = search.SortResults(m.merged, m.mode)
		if m.topN > 0 && len(m.results) > m.topN {
			m.results = m.results[:m.topN]
		}
		m.cursor[paneArticles], m.offset[paneArticles] = 0, 0
		return m.clampWindow(), nil

	case "/":
		// Xoá sạch ô nhập: vòng lặp thật là "đổi topic thử lại", nên gõ tiếp
		// vào topic cũ gần như luôn là ngoài ý muốn. Sửa topic cũ thì dùng `i`.
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
