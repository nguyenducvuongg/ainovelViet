package tui

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/nguyenducvuongg/ainovelViet/internal/host"
	"github.com/nguyenducvuongg/ainovelViet/internal/tools"
	"github.com/nguyenducvuongg/ainovelViet/internal/utils"
)

const maxEvents = 500

// maxStreamRounds giới hạn số vòng được bảng phát trực tuyến giữ lại. StreamClear được kích hoạt một lần vào cuối mỗi cuộc gọi LLM
// Mở một vòng mới, một người viết chương sẽ mất khoảng 3 ~ 5 vòng (tiêu đề tác nhân/suy nghĩ/bản nháp/cam kết), 32 vòng xấp xỉ bằng
// Nhìn lại kết quả phát trực tuyến của 6 ~ 10 chương gần đây. Văn bản của chương đã cam kết được lưu trữ trong kho/bản nháp. Nếu vượt quá giới hạn thì sẽ bị mất.
// Mỗi đồng bằng mã thông báo kích hoạt hiển thị lại O (toàn văn). Giới hạn trên của bộ nhớ trạng thái ổn định là khoảng 512KB, thấp hơn nhiều so với ngưỡng đóng băng.
const maxStreamRounds = 32

type focusPane int

const (
	focusEvents focusPane = iota
	focusStream
	focusDetail
	focusState // Thanh bên trạng thái bên trái (có thể cuộn)

	focusPaneCount // Tổng số tiêu điểm, được sử dụng để xoay Tab
)

type appMode int

const (
	modeNew     appMode = iota // Đợi người dùng nhập yêu cầu mới
	modeRunning                // Đang tạo (bao gồm cả việc dừng lỗi, có thể tiếp tục nhập liệu)
	modeDone                   // Quá trình tạo đã hoàn tất
)

// Chuỗi khung hình spinner (bubbles.Spinner.MiniDot) phổ biến cho các hoạt động phát trực tuyến/thanh trên cùng.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Một chuỗi các khung quay (bubbles.Spinner.Dot) dành riêng cho dòng "đang xử lý" của luồng sự kiện.
// 7 điểm + 1 khía xoay theo chiều kim đồng hồ dọc theo lưới 3×3, trông giống như một vòng tròn tải hoàn chỉnh.
// Sử dụng chỉ số khung độc lập + đánh dấu nhanh hơn mà không ảnh hưởng đến nhịp điệu của thanh trên cùng và hình ảnh động ngôi sao.
var toolSpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// Mô hình là trạng thái cấp cao nhất của TUI.
type Model struct {
	runtime        *host.Host
	askBridge      *askUserBridge
	askState       *askUserState
	cocreate       *cocreateState
	help           *helpState
	modelSwitch    *modelSwitchState
	report         *reportState
	version        string
	importer       *importState
	importSeq      int
	simulator      *simulationState
	simSeq         int
	compItems      []commandPaletteItem
	compIdx        int
	compActive     bool
	snapshot       host.UISnapshot
	events         []host.Event
	eventIndex     map[string]int   // sự kiện.ID → chỉ số m.events; cập nhật tại chỗ khi sự kiện lớp gọi đến
	viewport       viewport.Model   // khung nhìn luồng sự kiện
	streamVP       viewport.Model   // Truyền phát khung nhìn đầu ra
	detailVP       viewport.Model   // Khung nhìn chi tiết ở bên phải
	stateVP        viewport.Model   // Cổng xem thanh bên trạng thái bên trái (có thể cuộn)
	streamBuf      *strings.Builder // Bộ đệm tích lũy văn bản truyền phát
	streamRounds   []string
	textarea       textarea.Model
	width          int
	height         int
	autoScroll     bool
	streamScroll   bool      // Bảng điều khiển phát trực tuyến tự động theo sau
	streamDirty    bool      // StreamRounds có delta chưa được xóa; được hợp nhất bởi luồngFlushTick 60fps
	lastKeyAt      time.Time // Lần nhấn phím không phải Enter cuối cùng; KeyEnter điều chỉnh và chống dán \n Lỗi luồng kích hoạt gửi
	inputHistory   []string  // Lịch sử đầu vào đã gửi (loại bỏ trùng lặp: liền kề và không trùng lặp)
	historyIdx     int       // Chỉ mục duyệt web hiện tại; == len(inputHistory) có nghĩa là "chưa duyệt, đang chỉnh sửa bản nháp"
	historyDraft   string    // Các bản nháp đã lưu trước khi vào lịch sử duyệt web có thể được khôi phục khi quay lại phần cuối
	focusPane      focusPane
	hoverPane      focusPane
	hoverActive    bool
	mode           appMode
	startupMode    startupMode
	cocreateSeq    int
	reportSeq      int
	err            error
	spinnerIdx     int
	toolSpinnerIdx int  // Chỉ mục khung độc lập cho hàng đang diễn ra trong luồng sự kiện (đánh dấu 150 mili giây, không ảnh hưởng đến thanh/sao trên cùng)
	cursorIdx      int  // Chỉ số khung con trỏ truyền phát (đánh dấu độc lập)
	streamRound    int  // Truyền trực tiếp số vòng đầu ra
	quitPending    bool // Nhấn đúp Ctrl+C để thoát và xác nhận
	abortPending   bool // Tạm dừng thủ công chờ Done quay lại
	mouseOff       bool // Khi đúng, báo cáo bằng chuột bị tắt, cho phép người dùng kéo và thả để chọn và sao chép; chuyển đổi một lần nữa để khôi phục nó.
}

// NewModel tạo Mô hình TUI.
func NewModel(rt *host.Host, bridge *askUserBridge, version string) Model {
	ta := textarea.New()
	ta.Placeholder = placeholderForNewMode(startupModeQuick)
	ta.CharLimit = 2000
	ta.SetHeight(1)
	// MaxHeight=6 cho phép đầu vào quá dài được tự động gói và hiển thị thành nhiều dòng theo chiều rộng (giới hạn trên của hình ảnh là 6 dòng).
	ta.MaxHeight = 6
	ta.ShowLineNumbers = false
	ta.Focus()

	// Enter không được bao bọc theo mặc định (được gửi bởi handEnterKey);
	// Tự động liên kết lại dòng mới thành ctrl+j (unix \n) và alt+enter (quy ước GUI).
	// Lớp giao thức đầu cuối không thể phân biệt giữa Shift+Enter và Enter, do đó Shift+Enter không được hỗ trợ.
	ta.KeyMap.InsertNewline.SetKeys("ctrl+j", "alt+enter")

	vp := viewport.New(80, 20)
	vp.SetContent("")

	svp := viewport.New(80, 10)
	svp.SetContent("")

	dvp := viewport.New(40, 20)
	dvp.SetContent("")

	stvp := viewport.New(32, 20)
	stvp.SetContent("")

	return Model{
		runtime:      rt,
		askBridge:    bridge,
		version:      strings.TrimSpace(version),
		autoScroll:   true,
		streamScroll: true,
		mode:         modeNew,
		startupMode:  startupModeQuick,
		textarea:     ta,
		viewport:     vp,
		streamVP:     svp,
		detailVP:     dvp,
		stateVP:      stvp,
		streamBuf:    &strings.Builder{},
		eventIndex:   make(map[string]int),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		listenEvents(m.runtime),
		listenAskUser(m.askBridge),
		listenDone(m.runtime),
		listenStream(m.runtime),
		tickSnapshot(m.runtime),
		bootstrapRuntime(m.runtime),
		tickSpinner(),
		tickToolSpinner(),
		tickCursor(),
		tickStreamFlush(),
	)
}

func (m *Model) paneAtMouse(x, y int) (focusPane, bool) {
	if m.width == 0 || m.height == 0 {
		return focusEvents, false
	}

	topH, _, bodyH := m.layoutHeights()
	if bodyH < 1 {
		return focusEvents, false
	}

	bodyStartY := topH
	bodyEndY := topH + bodyH
	if y < bodyStartY || y >= bodyEndY {
		return focusEvents, false
	}

	leftW := m.sidebarWidth()
	rightW := m.detailWidth()
	centerStartX := leftW
	rightStartX := m.width - rightW

	if x >= rightStartX {
		return focusDetail, true
	}
	if x < centerStartX {
		return focusState, true
	}

	eventH, _ := m.splitHeights(bodyH)
	if y-bodyStartY < eventH {
		return focusEvents, true
	}
	return focusStream, true
}

func (m *Model) paneHighlighted(pane focusPane) bool {
	if m.focusPane == pane {
		return true
	}
	return m.hoverActive && m.hoverPane == pane
}

// hasRunningEvent Liệu có sự kiện gọi điện nào chưa hoàn thành (spinner vẫn đang quay) hay không.
// toolSpinnerTick sử dụng điều này để xác định xem có đáng hiển thị lại hay không: khi không có sự kiện đang chạy, khung spinner không ảnh hưởng đến đầu ra.
// Toàn bộ RefreshEventViewport chắc chắn không hoạt động.
func (m *Model) hasRunningEvent() bool {
	for i := range m.events {
		if m.events[i].Running() {
			return true
		}
	}
	return false
}

// FlushStreamIfDirty hiển thị luồng StreamRound tích lũy cho khung nhìn; dấu có nghĩa là đỏ bừng.
// Trả về xem nó có thực sự được chải hay không, để người gọi có thể quyết định có nên sử dụng GotoBottom hay không.
func (m *Model) flushStreamIfDirty() bool {
	if !m.streamDirty {
		return false
	}
	m.refreshStreamViewport()
	m.streamDirty = false
	return true
}

// RefreshEventViewport hiển thị lại nội dung luồng sự kiện và đặt chế độ xem.
func (m *Model) refreshEventViewport() {
	centerW := m.eventFlowWidth()
	content := renderEventContent(m.events, centerW, m.toolSpinnerIdx)
	if activity := renderEventActivity(m.snapshot, m.spinnerIdx, centerW); activity != "" {
		if strings.TrimSpace(content) != "" {
			content += "\n" + activity
		} else {
			content = activity
		}
	}
	m.viewport.SetContent(content)
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m *Model) refreshStreamViewport() {
	cursor := ""
	if m.snapshot.IsRunning {
		cursor = renderStreamCursor(m.cursorIdx)
	}
	m.streamVP.SetContent(renderStreamContent(m.streamRounds, m.streamVP.Width, cursor))
}

func (m *Model) refreshDetailViewport() {
	rightW := m.detailWidth()
	if rightW <= 4 {
		return
	}
	m.detailVP.SetContent(renderDetailContent(m.snapshot, rightW-4))
}

// RefreshStateViewport làm mới nội dung của thanh bên trạng thái bên trái vào khung nhìn.
// Nội dung thanh bên hoàn toàn bắt nguồn từ ảnh chụp nhanh nên phải được sơn lại khi ảnh chụp nhanh hoặc kích thước thay đổi.
func (m *Model) refreshStateViewport() {
	leftW := m.sidebarWidth()
	if leftW <= 4 {
		return
	}
	m.stateVP.SetContent(renderStateContent(m.snapshot, leftW-4))
}

// updateViewportSize cập nhật kích thước khung nhìn dựa trên kích thước cửa sổ hiện tại.
func (m *Model) updateViewportSize() {
	centerW := m.eventFlowWidth()
	rightW := m.detailWidth()
	bodyH := m.bodyHeight()
	eventH, streamH := m.splitHeights(bodyH)
	m.viewport.Width = centerW - 2
	m.viewport.Height = eventH - 1 // -1 là dòng tiêu đề của bảng sự kiện
	m.streamVP.Width = centerW - 2
	m.streamVP.Height = streamH - 1 // -1 là dòng tiêu đề của bảng điều khiển luồng
	m.detailVP.Width = rightW - 2
	m.detailVP.Height = bodyH
	leftW := m.sidebarWidth()
	m.stateVP.Width = max(1, leftW-2)
	m.stateVP.Height = max(1, bodyH-2) // -2 để lại khoảng trắng ở trên và dưới thanh trạng thái Padding(1,1)
}

// SplitHeights tính toán phân bổ chiều cao giữa các luồng sự kiện và đầu ra phát trực tuyến.
func (m *Model) splitHeights(bodyH int) (eventH, streamH int) {
	eventH = bodyH * 40 / 100
	if eventH < 3 {
		eventH = 3
	}
	streamH = bodyH - eventH - 1 // -1 là đường phân cách
	if streamH < 3 {
		streamH = 3
	}
	return
}

func (m *Model) inputWidth() int {
	if m.width == 0 {
		return 60
	}
	return m.width - 6 // đường viền + phần đệm + dấu nhắc "❯"
}

func (m *Model) currentInputWidth() int {
	if m.cocreate != nil {
		return coCreateInputWidth(m.width, m.height)
	}
	return m.inputWidth()
}

// refitTextareaHeight ước tính số lượng dòng trực quan dựa trên nội dung hiện tại, SetHeight động.
// Dòng trực quan = tổng các dòng logic (\n phần tách) mỗi đoạn được bao bọc bởi chiều rộng. Với MaxHeight=6
// Nhận ra "nội dung cực dài/hiển thị nhiều dòng tự động với gói dòng hoạt động, tối đa 6 dòng".
func (m *Model) refitTextareaHeight() {
	w := m.textarea.Width()
	if w <= 0 {
		return
	}
	// Trong chế độ đồng sáng tạo, đầu vào được cố định thành 1 dòng: nội dung nhiều dòng của vùng văn bản sẽ được chính vùng văn bản đó nhấn con trỏ.
	// Hiển thị cuộn. Nếu không, chiều cao của inputBox thay đổi theo nội dung, điều này sẽ khiến cuộc hội thoại ở cột bên trái bị thu hẹp lại.
	// Đầu vào trôi theo hướng thẳng đứng, phá hủy sự ổn định của bố cục.
	if m.cocreate != nil {
		m.textarea.SetHeight(1)
		return
	}
	text := m.textarea.Value()
	if text == "" {
		m.textarea.SetHeight(1)
		return
	}
	// 2 cột bị trừ (ký hiệu dấu nhắc bên trong vùng văn bản + con trỏ) và có thể chấp nhận thêm 1 hàng nữa.
	contentW := w - 2
	if contentW < 1 {
		contentW = 1
	}
	total := 0
	for line := range strings.SplitSeq(text, "\n") {
		lw := lipgloss.Width(line)
		if lw == 0 {
			total++
			continue
		}
		total += (lw + contentW - 1) / contentW
	}
	if total < 1 {
		total = 1
	}
	m.textarea.SetHeight(total) // SetHeight ép bên trong kẹp MaxHeight
}

// resizeTextarea đặt đồng bộ chiều rộng và chiều cao dựa trên nội dung.
// Thay thế các lệnh gọi SetWidth(currentInputWidth()) nằm rải rác khắp nơi để đảm bảo rằng chiều cao sẽ tuân theo khi chiều rộng thay đổi.
func (m *Model) resizeTextarea() {
	m.textarea.SetWidth(m.currentInputWidth())
	m.refitTextareaHeight()
}

// maxInputHistory giới hạn độ dài lịch sử để tránh tăng bộ nhớ trong các phiên dài.
const maxInputHistory = 200

// pushInputHistory Thêm nội dung đã gửi thành công vào lịch sử, loại bỏ trùng lặp. Đặt lại đồng bộ chỉ mục duyệt web.
func (m *Model) pushInputHistory(text string) {
	if text == "" {
		return
	}
	if n := len(m.inputHistory); n == 0 || m.inputHistory[n-1] != text {
		m.inputHistory = append(m.inputHistory, text)
		if len(m.inputHistory) > maxInputHistory {
			m.inputHistory = m.inputHistory[len(m.inputHistory)-maxInputHistory:]
		}
	}
	m.historyIdx = len(m.inputHistory)
	m.historyDraft = ""
}

// tryHistoryUp đi tới một lịch sử trước đó; trả về việc nhấn phím đã được xử lý hay chưa.
// Lưu nội dung vùng văn bản hiện tại dưới dạng bản nháp khi bạn truy cập lịch sử duyệt lần đầu tiên và khôi phục nội dung đó khi bạn quay lại phần cuối.
// Người gọi cần đánh giá xem có nên bỏ qua nó trong các tình huống nhiều dòng hay không (để vùng văn bản xử lý chuyển động của con trỏ trong dòng).
func (m *Model) tryHistoryUp() bool {
	if len(m.inputHistory) == 0 || m.historyIdx <= 0 {
		return false
	}
	if m.historyIdx == len(m.inputHistory) {
		m.historyDraft = m.textarea.Value()
	}
	m.historyIdx--
	m.textarea.SetValue(m.inputHistory[m.historyIdx])
	m.textarea.CursorEnd()
	m.refitTextareaHeight()
	return true
}

// tryHistoryDown sẽ cập nhật một phần lịch sử; đi đến cuối và khôi phục bản nháp.
func (m *Model) tryHistoryDown() bool {
	if m.historyIdx >= len(m.inputHistory) {
		return false
	}
	m.historyIdx++
	if m.historyIdx == len(m.inputHistory) {
		m.textarea.SetValue(m.historyDraft)
		m.historyDraft = ""
	} else {
		m.textarea.SetValue(m.inputHistory[m.historyIdx])
	}
	m.textarea.CursorEnd()
	m.refitTextareaHeight()
	return true
}

// textareaIsMultiline Liệu nội dung vùng văn bản hiện tại có chứa ngắt dòng hiện hoạt hay không; được sử dụng để xác định xem ↑↓ di chuyển trong lịch sử hay trong dòng.
func (m *Model) textareaIsMultiline() bool {
	return strings.Contains(m.textarea.Value(), "\n")
}

// inputHints tạo văn bản gợi ý dưới cùng dựa trên trạng thái hiện tại.
// Nối copySuffix thống nhất ở cuối để người dùng có thể thấy phương thức sao chép đã chọn trong bất kỳ trạng thái không khẩn cấp nào;
// Khi tắt chuột, hiển thị dòng chữ màu đỏ bắt mắt nhắc nhở bạn nhấn lại nút để tiếp tục tương tác chuột.
func (m *Model) inputHints() string {
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	if m.quitPending {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Bold(true).Render("Press Ctrl+C again to exit")
	}
	// Trang chào mừng (modeNew) không yêu cầu chuột để báo cáo và thiết bị đầu cuối có thể được sao chép bằng cách kéo nó nguyên bản mà không cần dấu nhắc Ctrl+R;
	// Bàn làm việc chỉ được mở để báo cáo và việc sao chép cần được đóng tạm thời bằng cách nhấn Ctrl+R.
	suffix := " · Ctrl+R chuyển sang chế độ sao chép đã chọn"
	if m.mode == modeNew {
		suffix = ""
	}
	if m.mouseOff && m.mode != modeNew {
		// Chuyển thủ công sang bản sao đã chọn trên bàn làm việc: sử dụng màu nhấn để cho biết trạng thái hiện tại là "lựa chọn kéo và thả tự do", nhấn Ctrl+R để khôi phục
		return lipgloss.NewStyle().Foreground(colorAccent).Bold(true).
			Render("✂ Chế độ sao chép đã chọn: Bạn có thể kéo và thả văn bản đã chọn để sao chép · Ctrl+R để thoát và khôi phục tương tác chuột")
	}
	if m.cocreate != nil {
		scrollHint := " · Cuộn tab: Hội thoại"
		if m.cocreate.focusPrompt {
			scrollHint = " · Cuộn tab: lệnh sáng tạo"
		}
		switch {
		case m.cocreate.awaiting:
			return dimStyle.Render("Đang chờ AI trả lời · Esc để thoát khỏi chế độ đồng sáng tạo" + scrollHint + suffix)
		case m.cocreate.canStart():
			startLabel := "Ctrl+S bắt đầu tạo"
			if m.cocreate.stage {
				startLabel = "Ctrl+S Áp dụng và tiếp tục"
			}
			return dimStyle.Render("Nhập Gửi · " + startLabel + " · Esc thoát khỏi chế độ đồng sáng tạo" + scrollHint + suffix)
		default:
			return dimStyle.Render("Enter để gửi · Esc để thoát đồng sáng tạo" + scrollHint + suffix)
		}
	}
	if m.mode == modeNew {
		if m.startupMode == startupModeQuick {
			return dimStyle.Render("Tab chuyển chế độ khởi động · Lệnh nhập / tìm kiếm · Enter bắt đầu tạo trực tiếp · Esc xóa đầu vào" + suffix)
		}
		return dimStyle.Render("Tab chuyển chế độ khởi động · Lệnh nhập/tìm kiếm · Enter bắt đầu cuộc trò chuyện đồng sáng tạo · Esc xóa đầu vào" + suffix)
	}
	switch m.snapshot.RuntimeState {
	case "pausing":
		return dimStyle.Render("Tạm dừng tạo · Vui lòng đợi kết thúc vòng hiện tại" + suffix)
	case "paused":
		return dimStyle.Render("Lệnh Enter/Tìm kiếm · Enter để tiếp tục tạo · Esc để xóa đầu vào" + suffix)
	}
	return dimStyle.Render("Lệnh nhập/tìm kiếm · Click/Tab để chuyển bảng · ↑↓ để cuộn · Kết thúc để nhảy xuống cuối · Ctrl+L để xóa màn hình · Esc để tạm dừng · Enter để gửi" + suffix)
}

func (m *Model) eventFlowWidth() int {
	if m.width == 0 {
		return 80
	}
	leftW := m.sidebarWidth()
	rightW := m.detailWidth()
	return m.width - leftW - rightW
}

func (m *Model) sidebarWidth() int {
	if m.width == 0 {
		return 32
	}
	return m.width * 23 / 100
}

func (m *Model) detailWidth() int {
	if m.width == 0 {
		return 40
	}
	return m.width * 27 / 100
}

func (m *Model) bodyHeight() int {
	_, _, bodyH := m.layoutHeights()
	return bodyH
}

func (m *Model) currentSpinnerFrame() string {
	if !m.snapshot.IsRunning {
		return ""
	}
	return spinnerFrames[m.spinnerIdx%len(spinnerFrames)]
}

func (m *Model) outputDir() string {
	if m.runtime == nil {
		return ""
	}
	return m.runtime.Dir()
}

func defaultSteerPlaceholder() string {
	return "Nhập can thiệp cốt truyện, ví dụ: tiến đường tình duyên đến Chương 4"
}

func (m *Model) syncRuntimePlaceholder() {
	if m.mode != modeRunning || m.cocreate != nil {
		return
	}
	switch m.snapshot.RuntimeState {
	case "completed":
		m.textarea.Placeholder = "Quá trình tạo đã hoàn tất"
	case "pausing":
		m.textarea.Placeholder = "Đang tạm dừng tạo..."
	case "paused":
		m.textarea.Placeholder = "Quá trình sáng tạo đã bị tạm dừng. Nhập nội dung bất kỳ để tiếp tục sáng tạo."
	default:
		if !m.snapshot.IsRunning {
			m.textarea.Placeholder = "Nếu thao tác bị gián đoạn, hãy nhập nội dung bất kỳ để tiếp tục tạo."
		} else {
			m.textarea.Placeholder = defaultSteerPlaceholder()
		}
	}
}

func (m *Model) renderBottomBar() string {
	inputBox := renderInputBox(
		m.textarea.View(),
		m.inputHints(),
		m.snapshot,
		m.outputDir(),
		m.width,
	)
	if m.mode != modeNew || m.cocreate != nil {
		return inputBox
	}
	return renderStartupModeBar(m.width, m.startupMode) + "\n" + inputBox
}

func (m *Model) layoutHeights() (topH, inputH, bodyH int) {
	if m.width == 0 || m.height == 0 {
		return 1, 4, 20
	}
	topH = lipgloss.Height(renderTopBar(m.snapshot, m.width, m.currentSpinnerFrame(), m.version))
	inputH = lipgloss.Height(m.renderBottomBar())
	bodyH = m.height - topH - inputH
	if bodyH < 3 {
		bodyH = 3
	}
	return
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "đang tải..."
	}
	if m.width < 100 {
		return lipgloss.NewStyle().
			Width(m.width).Height(m.height).
			AlignHorizontal(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Render("Độ rộng của thiết bị đầu cuối không đủ, vui lòng mở rộng lên ít nhất 100 cột")
	}
	if m.askState != nil {
		return renderAskUserModal(m.width, m.height, m.askState)
	}
	if m.cocreate != nil {
		return renderCoCreateModal(m.width, m.height, m.cocreate, errorText(m.err), m.textarea.View(), m.spinnerIdx, m.quitPending)
	}
	if m.help != nil {
		return renderHelpModal(m.width, m.height, m.help)
	}
	if m.report != nil {
		return renderReportModal(m.width, m.height, m.report)
	}
	if m.importer != nil {
		return renderImportModal(m.width, m.height, m.importer)
	}
	if m.simulator != nil {
		return renderSimulationModal(m.width, m.height, m.simulator)
	}

	topBar := renderTopBar(m.snapshot, m.width, m.currentSpinnerFrame(), m.version)
	inputBox := m.renderBottomBar()
	_, inputH, bodyH := m.layoutHeights()

	var body string
	if m.mode == modeNew {
		errMsg := ""
		if m.err != nil {
			errMsg = m.err.Error()
		}
		body = renderWelcome(m.width, bodyH, errMsg, m.startupMode)
	} else {
		leftW := m.sidebarWidth()
		rightW := m.detailWidth()
		centerW := m.width - leftW - rightW
		eventH, streamH := m.splitHeights(bodyH)

		if m.viewport.Width != centerW-2 || m.viewport.Height != eventH-1 {
			m.viewport.Width = centerW - 2
			m.viewport.Height = eventH - 1 // -1 là dòng tiêu đề của bảng sự kiện
		}
		if m.streamVP.Width != centerW-2 || m.streamVP.Height != streamH-1 {
			m.streamVP.Width = centerW - 2
			m.streamVP.Height = streamH - 1 // -1 là dòng tiêu đề của bảng điều khiển luồng
		}

		eventFlow := renderEventFlowViewport(m.viewport, centerW, eventH, m.paneHighlighted(focusEvents))
		streamPanel := renderStreamPanel(m.streamVP, centerW, streamH, m.paneHighlighted(focusStream), m.snapshot.IsRunning, m.spinnerIdx)
		center := lipgloss.JoinVertical(lipgloss.Left, eventFlow, streamPanel)

		left := renderStatePanel(m.stateVP, leftW, bodyH, m.paneHighlighted(focusState))
		right := renderDetailPanel(m.detailVP, rightW, bodyH, m.paneHighlighted(focusDetail))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, topBar, body, inputBox)

	// Lớp phủ cửa sổ bật lên: nổi phía trên phần dưới cùng của nội dung mà không ảnh hưởng đến bố cục
	if m.modelSwitch != nil {
		commandBar := renderModelSwitchBar(m.width, m.modelSwitch)
		view = overlayAboveInput(view, commandBar, inputH)
	} else if m.compActive {
		commandBar := renderCommandPalette(m.width, m.compItems, m.compIdx)
		view = overlayAboveInput(view, commandBar, inputH)
	}
	return view
}

// sendCoCreate bắt đầu một vòng yêu cầu đồng sáng tạo và xử lý reqID, vùng văn bản và trình giữ chỗ theo cách thống nhất.
func (m *Model) sendCoCreate() tea.Cmd {
	m.cocreateSeq++
	m.cocreate.reqID = m.cocreateSeq
	m.cocreate.awaiting = true
	m.resizeTextarea()
	m.textarea.Placeholder = placeholderForCoCreate(m.cocreate)
	m.textarea.Blur()
	return runCoCreate(m.runtime, m.cocreate)
}

func (m Model) handleCoCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cocreate == nil {
		return m, nil
	}
	state := m.cocreate

	// Bàn phím ↑↓/PgUp/PgDn/Home/End cuộn; Tab chuyển tiêu điểm cuộn giữa thanh hội thoại bên trái ↔ thanh lệnh quảng cáo bên phải
	// (Mặc định cột bên trái, người dùng nhìn lại nội dung chính). Trang chào mừng đã tắt báo cáo chuột để giữ lại bản gốc, khi cột bên phải tràn ra nhấn Tab
	// Sử dụng bàn phím để cuộn sau khi chuyển tiêu điểm. Cột bên trái: Kéo lên để tắt theo dõi, kéo xuống phía dưới để bật lại theo dõi (streaming follow).
	switch msg.Type {
	case tea.KeyTab:
		state.focusPrompt = !state.focusPrompt
		return m, nil
	case tea.KeyUp, tea.KeyPgUp:
		if state.focusPrompt {
			var cmd tea.Cmd
			state.promptVP, cmd = state.promptVP.Update(msg)
			return m, cmd
		}
		state.convFollow = false
		var cmd tea.Cmd
		state.convVP, cmd = state.convVP.Update(msg)
		return m, cmd
	case tea.KeyDown, tea.KeyPgDown:
		if state.focusPrompt {
			var cmd tea.Cmd
			state.promptVP, cmd = state.promptVP.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		state.convVP, cmd = state.convVP.Update(msg)
		if state.convVP.AtBottom() {
			state.convFollow = true
		}
		return m, cmd
	case tea.KeyHome:
		if state.focusPrompt {
			state.promptVP.GotoTop()
			return m, nil
		}
		state.convFollow = false
		state.convVP.GotoTop()
		return m, nil
	case tea.KeyEnd:
		if state.focusPrompt {
			state.promptVP.GotoBottom()
			return m, nil
		}
		state.convFollow = true
		state.convVP.GotoBottom()
		return m, nil
	case tea.KeyEsc:
		return m.exitCoCreate()
	}

	// Giải phóng lớp chỉnh sửa (nhập ký tự/xóa lùi/con trỏ/Ctrl+U/ngắt dòng nhiều dòng) trong khi chờ AI trả lời——
	// Người dùng có thể gõ trước câu tiếp theo trong khi AI đang suy nghĩ. Sự che chắn của lớp đệ trình chìm vào từng trường hợp.
	// Hãy Enter ga trước khi chờ - bằng cách này, đoạn \n đã dán vẫn có thể lấp đầy khoảng trống.

	switch msg.Type {
	case tea.KeyCtrlS:
		if state.awaiting {
			return m, nil
		}
		if !state.canStart() {
			return m, nil
		}
		// Đồng sáng tạo theo giai đoạn: Đưa vào "Bản tóm tắt hướng dẫn tiếp theo" và tiếp tục quá trình tạo, quay lại thời gian chạy.
		if state.stage {
			draft := state.draftPrompt()
			m.cocreate = nil
			m.err = nil
			m.resizeTextarea()
			m.textarea.Placeholder = defaultSteerPlaceholder()
			return m, tea.Batch(resumeFromCoCreate(m.runtime, draft), m.textarea.Focus())
		}
		// Đồng sáng tạo bắt đầu từ đầu: Bắt đầu sáng tạo với các hướng dẫn sáng tạo có tổ chức.
		plan, err := state.buildPlan()
		if err != nil {
			m.err = err
			return m, nil
		}
		state.awaiting = true
		m.textarea.Blur()
		return m, startRuntime(m.runtime, plan)
	case tea.KeyEnter:
		// Alt+Enter → Tự động ngắt dòng và để textarea.Update tiếp quản (KeyMap.InsertNewline đã được liên kết với khóa này)
		if msg.Alt {
			break
		}
		// Khoảng cách từ lần nhấn phím ký tự cuối cùng quá ngắn → được coi là \n đoạn của luồng dán: hãy điền vào khoảng trắng thay vì gửi.
		// Nó phải được đánh giá trước khi chờ đợi bị chặn - nếu không các đoạn \n được dán trong quá trình chờ đợi sẽ bị chặn.
		// Kết quả là, "abc\ndef" bị nuốt vào "abcdef", điều này không phù hợp với ngữ nghĩa đường dẫn cơ sở.
		if !m.lastKeyAt.IsZero() && time.Since(m.lastKeyAt) < 50*time.Millisecond {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
			m.refitTextareaHeight()
			return m, cmd
		}
		// Ý định gửi thực sự: được bảo vệ trong thời gian chờ đợi (không thể gửi yêu cầu đồng thời)
		if state.awaiting {
			return m, nil
		}
		text := utils.CleanInputLine(m.textarea.Value())
		if text == "" {
			return m, nil
		}
		m.err = nil
		state.appendUser(text)
		m.textarea.Reset()
		m.refitTextareaHeight()
		cmd := m.sendCoCreate()
		return m, cmd
	case tea.KeyCtrlU:
		m.textarea.Reset()
		m.refitTextareaHeight()
		return m, nil
	}

	// Phím số 1/2/3 Khi vùng văn bản trống và có gợi ý → điền gợi ý tương ứng (không gửi được, có thể chỉnh sửa).
	// Chỉ chặn khi ô nhập trống để tránh ảnh hưởng đến thao tác gõ số đang hoạt động của người dùng. Khuyến cáo không nên hiển thị khi chờ đợi.
	// Không cần phải đánh giá bổ sung ở đây (state.suggestions sẽ bị bỏ qua nếu nó trống).
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && !state.awaiting {
		if r := msg.Runes[0]; r >= '1' && r <= '3' {
			if strings.TrimSpace(m.textarea.Value()) == "" {
				if sugs := state.suggestions(); int(r-'0') <= len(sugs) {
					m.textarea.SetValue(sugs[r-'1'])
					m.refitTextareaHeight()
					return m, nil
				}
			}
		}
	}

	// Đầu vào thông thường được chuyển tiếp tới vùng văn bản
	if msg.Type == tea.KeyRunes && (containsSGRFragment(string(msg.Runes)) || isCSILeak(msg.Runes)) {
		return m, nil
	}
	var ok bool
	if msg, ok = cleanHumanKeyRunes(msg); !ok {
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.lastKeyAt = time.Now()
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.refitTextareaHeight()
	return m, cmd
}

// exitCoCreate thoát chế độ đồng sáng tạo, hủy yêu cầu LLM đang diễn ra và khôi phục trạng thái hộp đầu vào.
func (m Model) exitCoCreate() (tea.Model, tea.Cmd) {
	if m.cocreate.cancel != nil {
		m.cocreate.cancel()
	}
	stage := m.cocreate.stage
	initial := m.cocreate.initialInput()
	m.cocreate = nil
	m.resizeTextarea()
	// Hủy đồng sáng tạo giai đoạn: xóa dấu nghề nghiệp, giữ tạm dừng và quay lại trạng thái đầu vào của bảng đang chạy (không chèn lấp phần mở tổng hợp).
	if stage {
		m.textarea.SetValue("")
		m.textarea.Placeholder = defaultSteerPlaceholder()
		return m, tea.Batch(cancelCoCreate(m.runtime), fetchSnapshot(m.runtime), m.textarea.Focus())
	}
	m.textarea.SetValue(initial)
	m.textarea.Placeholder = placeholderForNewMode(m.startupMode)
	return m, m.textarea.Focus()
}

func (m Model) handleAskUserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.askState == nil {
		return m, nil
	}
	state := m.askState
	q := state.currentQuestion()

	if state.typing {
		switch msg.Type {
		case tea.KeyEsc:
			state.cancelCurrentTyping()
			return m, nil
		case tea.KeyEnter:
			if state.finishCurrentAnswer() {
				state.submit()
				m.askState = nil
				return m, m.textarea.Focus()
			}
			return m, nil
		case tea.KeyBackspace, tea.KeyCtrlH:
			if state.input != "" {
				_, size := utf8.DecodeLastRuneInString(state.input)
				state.input = state.input[:len(state.input)-size]
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				state.input += utils.CleanInputRunes(msg.Runes)
			}
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyEsc:
		// Đóng cửa sổ bật lên và trả về câu trả lời trống
		state.request.resultCh <- askUserResult{
			resp: &tools.AskUserResponse{
				Answers: make(map[string]string),
				Notes:   make(map[string]string),
			},
		}
		m.askState = nil
		return m, m.textarea.Focus()
	case tea.KeyUp:
		state.moveCursor(-1)
	case tea.KeyDown:
		state.moveCursor(1)
	case tea.KeySpace:
		if q.MultiSelect {
			state.toggleSelection()
			if state.cursor == len(q.Options) && !state.selected[state.cursor] {
				state.input = ""
			}
		}
	case tea.KeyEnter:
		if q.MultiSelect {
			if state.cursor == len(q.Options) {
				state.toggleSelection()
				if state.selected[state.cursor] {
					state.typing = true
				}
				return m, nil
			}
			if len(state.selected) == 0 {
				state.toggleSelection()
			}
		}
		if state.finishCurrentAnswer() {
			state.submit()
			m.askState = nil
			return m, m.textarea.Focus()
		}
	}
	return m, nil
}

// lớp phủAboveInput sẽ làm nổi lớp phủ ở cuối chế độ xem cơ sở (phía trên hộp đầu vào),
// Không thay đổi chiều cao bố cục tổng thể. Chỉ có chiều rộng của thẻ lớp phủ được che đi và nội dung cơ bản được hiển thị ở phía bên phải.
func overlayAboveInput(base, overlay string, inputLineCount int) string {
	baseLines := strings.Split(base, "\n")
	overLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")

	endY := len(baseLines) - inputLineCount
	startY := endY - len(overLines)
	if startY < 0 {
		startY = 0
	}

	for i, ol := range overLines {
		y := startY + i
		if y >= 0 && y < endY {
			olW := lipgloss.Width(ol)
			// Cắt bỏ các ký tự hiển thị olW ở phía bên trái của đường cơ sở và ghép lớp phủ + nội dung bên phải còn lại
			right := ansi.TruncateLeft(baseLines[y], olW, "")
			baseLines[y] = ol + right
		}
	}
	return strings.Join(baseLines, "\n")
}

// isCSILeak phát hiện xem KeyRunes có phải là các đoạn rò rỉ chuỗi thoát CSI hay không.
// Khi thiết bị đầu cuối gửi các phím mũi tên \x1b[A, việc nhấn phím nhanh có thể khiến chuỗi bị phân tách:
// \x1b được phân tích cú pháp dưới dạng Escape và "[" hoặc "[A" bị rò rỉ ra vùng văn bản dưới dạng KeyRunes.
func isCSILeak(runes []rune) bool {
	if len(runes) == 0 || runes[0] != '[' {
		return false
	}
	for _, r := range runes[1:] {
		if (r >= '0' && r <= '9') || r == ';' ||
			(r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '~' {
			continue
		}
		return false
	}
	return true
}

// chứaSGRFragment Kiểm tra xem văn bản có chứa đoạn chuỗi chuột SGR hay không (mẫu ("<number;number;").
func containsSGRFragment(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		j := i + 1
		if j >= len(s) || s[j] < '0' || s[j] > '9' {
			continue
		}
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j < len(s) && s[j] == ';' {
			return true
		}
	}
	return false
}

func cleanHumanKeyRunes(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		return msg, true
	}
	cleaned := utils.CleanInputRunes(msg.Runes)
	if cleaned == "" {
		return msg, false
	}
	msg.Runes = []rune(cleaned)
	return msg, true
}
