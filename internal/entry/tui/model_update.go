package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/entry/startup"
	"github.com/nguyenducvuongg/ainovelViet/internal/host"
	"github.com/nguyenducvuongg/ainovelViet/internal/host/imp"
	"github.com/nguyenducvuongg/ainovelViet/internal/utils"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeTextarea()
		m.updateViewportSize()
		m.refreshDetailViewport()
		m.refreshStateViewport()
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.MouseMsg:
		return m.handleMouseMsg(msg)
	default:
		if next, cmd, handled := m.handleRuntimeMsg(msg); handled {
			return next, cmd
		}
		return m.handleTextareaMsg(msg)
	}
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleOverlayKeyMsg(msg); handled {
		return next, cmd
	}

	if msg.Type == tea.KeyCtrlC {
		if m.quitPending {
			return m, tea.Quit
		}
		m.quitPending = true
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return quitResetMsg{} })
	}
	m.quitPending = false

	if next, cmd, handled := m.handleCommandPaletteKey(msg); handled {
		return next, cmd
	}

	return m.handleBaseKeyMsg(msg)
}

func (m Model) handleOverlayKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case m.askState != nil:
		return m.handleBlockingModalKey(msg, m.handleAskUserKey)
	case m.cocreate != nil:
		return m.handleBlockingModalKey(msg, m.handleCoCreateKey)
	case m.help != nil:
		return m.handleBlockingModalKey(msg, m.handleHelpKey)
	case m.modelSwitch != nil:
		return m.handleBlockingModalKey(msg, m.handleModelSwitchKey)
	case m.report != nil:
		return m.handleBlockingModalKey(msg, m.handleReportKey)
	case m.importer != nil:
		return m.handleBlockingModalKey(msg, m.handleImportKey)
	case m.simulator != nil:
		return m.handleBlockingModalKey(msg, m.handleSimulationKey)
	default:
		return m, nil, false
	}
}

func (m Model) handleBlockingModalKey(msg tea.KeyMsg, next func(tea.KeyMsg) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd, bool) {
	if msg.Type == tea.KeyCtrlC {
		if m.quitPending {
			return m, tea.Quit, true
		}
		m.quitPending = true
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return quitResetMsg{} }), true
	}
	m.quitPending = false
	// Phím tắt chung đa phương thức: Bạn phải có khả năng xoay chuột để báo cáo trong khi phương thức đang mở, nếu không, bạn sẽ tạo /help/report, v.v.
	// Người dùng không thể chọn và sao chép bằng cách kéo và thả gốc trong chế độ màn hình khóa.
	if msg.Type == tea.KeyCtrlR {
		next, cmd := m.toggleMouseReporting()
		return next, cmd, true
	}
	model, cmd := next(msg)
	return model, cmd, true
}

// chuyển đổiMouseReporting bật và tắt tính năng báo cáo bằng chuột. Bật → Tắt cho phép người dùng kéo, chọn và sao chép nguyên bản;
// Tắt → Bật Khôi phục Nhấp để chuyển tiêu điểm/bánh xe cuộn. Đường dẫn cơ sở được chia sẻ với đường dẫn phương thức chặn.
func (m Model) toggleMouseReporting() (Model, tea.Cmd) {
	// Trang chào mừng (modeNew) không cho phép báo cáo bằng chuột và có thể được sao chép bằng cách kéo nó nguyên bản; Ctrl+R bị bỏ qua ở đây.
	// Tránh vô tình mở báo cáo và phá hủy bản sao gốc. Leo thang chuột được bật bằng enterRunning khi vào bàn làm việc.
	if m.mode == modeNew {
		return m, nil
	}
	m.mouseOff = !m.mouseOff
	if m.mouseOff {
		return m, tea.DisableMouse
	}
	return m, tea.EnableMouseCellMotion
}

// enterRunning Vào bàn làm việc sáng tạo: bật báo cáo bằng chuột (bàn làm việc cần nhấp vào bảng cắt/bánh xe/
// Kéo thanh bên). Lệnh trả về cần được người gọi đưa vào giá trị trả về cuối cùng.
func (m *Model) enterRunning() tea.Cmd {
	m.mode = modeRunning
	m.mouseOff = false
	return tea.EnableMouseCellMotion
}

func (m Model) handleCommandPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if !m.compActive {
		return m, nil, false
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.clearCommandPalette()
		return m, nil, true
	case tea.KeyUp:
		if m.compIdx > 0 {
			m.compIdx--
		}
		return m, nil, true
	case tea.KeyDown:
		if m.compIdx < len(m.compItems)-1 {
			m.compIdx++
		}
		return m, nil, true
	case tea.KeyTab:
		m.acceptCommandCompletion()
		return m, nil, true
	case tea.KeyEnter:
		item, ok := m.acceptCommandCompletion()
		if !ok {
			return m, nil, true
		}
		if item.AutoExecute {
			m.textarea.Reset()
			next, cmd := m.handleSlashCommand(slashCommand{name: item.Name})
			return next, cmd, true
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleBaseKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Phòng thủ điều tiết: Dán \n sẽ thoái hóa thành KeyEnter liên tục trong các thiết bị đầu cuối không hỗ trợ dán trong ngoặc;
	// Khoảng thời gian giữa người thật nhấn Enter và ký tự trước đó thường là >100 mili giây và <50 mili giây rất có thể là một đoạn của luồng dán.
	// Chỉ cần nhớ KeyRunes (luồng ký tự) - các phím chức năng (↑↓/Tab/Ctrl-x) không được làm ảnh hưởng đến việc điều tiết,
	// Nếu không, người dùng sẽ vô tình bị nuốt chửng khi nhấn Enter ngay sau khi cuộn qua lựa chọn lịch sử.
	if msg.Type == tea.KeyRunes {
		m.lastKeyAt = time.Now()
	}
	switch msg.Type {
	case tea.KeyEscape:
		if m.mode == modeRunning && m.snapshot.IsRunning {
			return m, abortRuntime(m.runtime)
		}
		m.textarea.Reset()
		m.historyIdx = len(m.inputHistory)
		m.historyDraft = ""
		m.refitTextareaHeight()
		m.clearCommandPalette()
		return m, nil
	case tea.KeyCtrlL:
		m.resetOutputPanels()
		return m, nil
	case tea.KeyCtrlU:
		// Xóa đầu vào hiện tại; thoát khỏi trạng thái duyệt lịch sử cùng một lúc.
		m.textarea.Reset()
		m.historyIdx = len(m.inputHistory)
		m.historyDraft = ""
		m.refitTextareaHeight()
		m.clearCommandPalette()
		return m, nil
	case tea.KeyCtrlR:
		return m.toggleMouseReporting()
	case tea.KeyTab:
		if m.mode == modeNew {
			if m.cocreate != nil {
				return m, nil
			}
			if m.startupMode == startupModeQuick {
				m.startupMode = startupModeCoCreate
			} else {
				m.startupMode = startupModeQuick
			}
			m.textarea.Placeholder = placeholderForNewMode(m.startupMode)
			return m, nil
		}
		m.focusPane = (m.focusPane + 1) % focusPaneCount
		return m, nil
	case tea.KeyEnter:
		// Alt+Enter tự động ngắt dòng, để textarea.Update tiếp quản (KeyMap.InsertNewline đã được gắn với khóa này).
		if msg.Alt {
			break
		}
		// Khoảng thời gian giữa lần nhấn phím không phải Enter cuối cùng quá ngắn → được coi là \n đoạn của luồng dán:
		// Việc thay thế bằng dấu cách sẽ duy trì khoảng cách trực quan, nhất quán với ngữ nghĩa đường dẫn cleanHumanKeyRunes ("abc\ndef" → "abc def").
		// Bảo vệ chống lại các môi trường thiết bị đầu cuối bị hỏng dán trong ngoặc (cấu hình SSH/tmux cũ nhất định).
		if !m.lastKeyAt.IsZero() && time.Since(m.lastKeyAt) < 50*time.Millisecond {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
			m.refitTextareaHeight()
			return m, cmd
		}
		return m.handleEnterKey()
	case tea.KeyUp:
		// Đầu vào nhiều dòng: để vùng văn bản đảm nhận chuyển động của con trỏ trong dòng (rơi vào vùng văn bản. Cập nhật sau khi chuyển đổi)
		if m.textareaIsMultiline() {
			break
		}
		// Một dòng: cuộn qua lịch sử trước, quay lại cuộn luồng sự kiện khi không có lịch sử
		if m.tryHistoryUp() {
			return m, nil
		}
		return m.handleVerticalScrollKey(msg, true)
	case tea.KeyDown:
		if m.textareaIsMultiline() {
			break
		}
		if m.tryHistoryDown() {
			return m, nil
		}
		return m.handleVerticalScrollKey(msg, false)
	case tea.KeyPgUp:
		return m.handleVerticalScrollKey(msg, true)
	case tea.KeyPgDown:
		return m.handleVerticalScrollKey(msg, false)
	case tea.KeyEnd:
		switch m.focusPane {
		case focusStream:
			m.streamScroll = true
			m.streamVP.GotoBottom()
		case focusDetail:
			m.detailVP.GotoBottom()
		case focusState:
			m.stateVP.GotoBottom()
		default:
			m.autoScroll = true
			m.viewport.GotoBottom()
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes && (containsSGRFragment(string(msg.Runes)) || isCSILeak(msg.Runes)) {
		return m, nil
	}
	var ok bool
	if msg, ok = cleanHumanKeyRunes(msg); !ok {
		return m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.refitTextareaHeight()
	m.updateCommandPalette()
	return m, cmd
}

func (m Model) handleEnterKey() (tea.Model, tea.Cmd) {
	text := utils.CleanInputLine(m.textarea.Value())
	if text == "" {
		return m, nil
	}
	m.clearCommandPalette()
	if cmd, ok := parseSlashCommand(text); ok {
		m.pushInputHistory(text)
		m.textarea.Reset()
		m.refitTextareaHeight()
		return m.handleSlashCommand(cmd)
	}

	m.pushInputHistory(text)
	m.textarea.Reset()
	m.refitTextareaHeight()
	switch m.mode {
	case modeNew:
		m.err = nil
		if m.startupMode == startupModeQuick {
			plan, err := startup.PrepareQuick(startup.Request{
				Mode:        startup.ModeQuick,
				UserPrompt:  text,
				OutputDir:   m.runtime.Dir(),
				Interactive: true,
			})
			if err != nil {
				m.err = err
				return m, nil
			}
			return m, startRuntime(m.runtime, plan)
		}
		m.cocreate = newCoCreateState(text)
		return m, m.sendCoCreate()
	case modeRunning:
		// Các sự kiện USER không được lặp lại cục bộ - mục Host.Continue/Steer đã phát ra sự kiện "USER",
		// Sử dụng kênh sự kiện để quay trở lại TUI. Kiến trúc §2.3: Lớp quan sát chỉ quan sát chứ không đưa ra dữ kiện.
		if !m.snapshot.IsRunning {
			return m, continueRuntime(m.runtime, text)
		}
		return m, steerRuntime(m.runtime, text)
	case modeDone:
		// Sau khi hoàn thành, người dùng nhập (yêu cầu làm lại/tiếp tục): đánh thức một vòng chạy mới. Tiếp tục ở trạng thái dừng và đi đến Tiêm
		// Tự động khôi phục, Điều phối viên nhận được [Can thiệp của người dùng] và các tuyến theo điều phối viên.md - Yêu cầu làm lại đã được ký
		// Sau đó điều chỉnh open_book để mở lại sách ở trạng thái làm lại. Chuyển về chế độChạy và vào lại bàn làm việc; vòng này đã hoàn thành
		// doneMsg(complete) sẽ đặt lại chế độDone. Các lệnh gạch chéo được xử lý trước ở trên và không đi qua nhánh này.
		m.mode = modeRunning
		return m, continueRuntime(m.runtime, text)
	default:
		return m, nil
	}
}

func (m Model) handleVerticalScrollKey(msg tea.KeyMsg, upward bool) (tea.Model, tea.Cmd) {
	if m.focusPane == focusStream {
		if upward {
			m.streamScroll = false
		}
		var cmd tea.Cmd
		m.streamVP, cmd = m.streamVP.Update(msg)
		if !upward && m.streamVP.AtBottom() {
			m.streamScroll = true
		}
		return m, cmd
	}
	if m.focusPane == focusDetail {
		var cmd tea.Cmd
		m.detailVP, cmd = m.detailVP.Update(msg)
		return m, cmd
	}
	if m.focusPane == focusState {
		var cmd tea.Cmd
		m.stateVP, cmd = m.stateVP.Update(msg)
		return m, cmd
	}
	if upward {
		m.autoScroll = false
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	if !upward && m.viewport.AtBottom() {
		m.autoScroll = true
	}
	return m, cmd
}

func (m Model) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.cocreate != nil {
		// Chuột được chia theo tọa độ X: nửa bên trái của màn hình = bảng chuyển đổi, nửa bên phải = bảng nhắc nhở.
		// Phương thức được căn giữa và đối lưu chiếm ~58% bên trái. Phán đoán sử dụng đường trung tâm màn hình là đủ chính xác.
		// Con lăn của người dùng tự động dừng theo dõi trong vùng đối lưu (cho phép dừng ổn định ở một vị trí lịch sử nhất định).
		var cmd tea.Cmd
		if msg.X < m.width/2 {
			m.cocreate.convFollow = false
			m.cocreate.convVP, cmd = m.cocreate.convVP.Update(msg)
			if m.cocreate.convVP.AtBottom() {
				m.cocreate.convFollow = true
			}
		} else {
			m.cocreate.promptVP, cmd = m.cocreate.promptVP.Update(msg)
		}
		return m, cmd
	}
	if m.modelSwitch != nil || m.askState != nil {
		return m, nil
	}
	if pane, ok := m.paneAtMouse(msg.X, msg.Y); ok {
		m.hoverPane = pane
		m.hoverActive = true
		if msg.Action == tea.MouseActionPress {
			m.focusPane = pane
		}
	} else {
		m.hoverActive = false
	}

	var cmd tea.Cmd
	if m.focusPane == focusStream {
		m.streamVP, cmd = m.streamVP.Update(msg)
		if msg.Action == tea.MouseActionPress {
			m.streamScroll = m.streamVP.AtBottom()
		}
		return m, cmd
	}
	if m.focusPane == focusDetail {
		m.detailVP, cmd = m.detailVP.Update(msg)
		return m, cmd
	}
	if m.focusPane == focusState {
		m.stateVP, cmd = m.stateVP.Update(msg)
		return m, cmd
	}
	m.viewport, cmd = m.viewport.Update(msg)
	if msg.Action == tea.MouseActionPress {
		m.autoScroll = m.viewport.AtBottom()
	}
	return m, cmd
}

func (m Model) handleRuntimeMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case eventMsg:
		ev := host.Event(msg)
		m.applyEvent(ev)
		m.refreshEventViewport()
		return m, listenEvents(m.runtime), true
	case bootstrapMsg:
		// Phát lại các sự kiện lịch sử trước rồi xử lý lỗi: Việc từ chối tiếp tục (chẳng hạn như giới hạn ngân sách) là cách thông thường.
		// Người dùng cần có khả năng đọc được lý do từ chối trong khi xem lịch sử, thay vì phải đối mặt với luồng sự kiện trống.
		m.applyRuntimeReplay(msg.replay)
		if msg.err != nil {
			m.err = msg.err
			return m, fetchSnapshot(m.runtime), true
		}
		if msg.resumed && m.mode == modeNew {
			enableMouse := m.enterRunning()
			m.resizeTextarea()
			m.textarea.Placeholder = defaultSteerPlaceholder()
			return m, tea.Batch(fetchSnapshot(m.runtime), enableMouse), true
		}
		return m, fetchSnapshot(m.runtime), true
	case askUserMsg:
		m.askState = newAskUserState(askUserRequest(msg))
		m.textarea.Blur()
		m.applyEvent(host.Event{
			Time: time.Now(), Category: "SYSTEM", Summary: "Đợi người dùng thêm thông tin chính", Level: "info",
		})
		m.refreshEventViewport()
		return m, listenAskUser(m.askBridge), true
	case snapshotMsg:
		m.snapshot = host.UISnapshot(msg)
		m.syncRuntimePlaceholder()
		m.refreshEventViewport()
		m.refreshStreamViewport()
		m.refreshDetailViewport()
		m.refreshStateViewport()
		return m, tickSnapshot(m.runtime), true
	case doneMsg:
		m.snapshot.IsRunning = false
		m.refreshEventViewport()
		m.refreshStreamViewport()
		m.refreshStateViewport()
		if msg.complete {
			m.abortPending = false
			m.mode = modeDone
			// Hộp nhập không bị khóa ở trạng thái hoàn thành: việc tự động gia hạn bị dừng nhưng người dùng vẫn có thể nhập các yêu cầu làm lại (chế độNhập xong
			// Tiếp tục đánh thức vòng chạy mới, Điều phối viên định tuyến tới open_book), /export, /model
			// Các lệnh khác cũng phải có sẵn và hộp nhập phải vẫn được tập trung (vấn đề #27, #38).
			m.textarea.Placeholder = "Quá trình tạo đã hoàn tất · Bạn có thể nhập các yêu cầu làm lại (chẳng hạn như \" viết lại Chương 3 \"), /xuất xuất hoặc nhập / để xem lệnh"
			return m, tea.Batch(fetchSnapshot(m.runtime), listenDone(m.runtime), m.textarea.Focus()), true
		}
		if m.abortPending {
			m.abortPending = false
			m.snapshot.RuntimeState = "paused"
			m.syncRuntimePlaceholder()
		} else {
			m.textarea.Placeholder = "Nếu thao tác bị gián đoạn, hãy nhập nội dung bất kỳ để tiếp tục tạo."
		}
		return m, tea.Batch(fetchSnapshot(m.runtime), listenDone(m.runtime)), true
	case abortResultMsg:
		if msg.stopped {
			m.abortPending = true
			m.textarea.Placeholder = "Đang tạm dừng tạo..."
		}
		return m, nil, true
	case reportLoadedMsg:
		if m.report == nil || msg.reqID != m.report.reqID {
			return m, nil, true
		}
		boxW, _ := reportModalSize(m.width, m.height)
		m.report.load(msg.report, paddedModalContentWidth(boxW), msg.exportPath, msg.finishedAt)
		return m, nil, true
	case importEventMsg:
		if m.importer == nil || msg.reqID != m.importer.reqID {
			return m, nil, true
		}
		boxW, _ := reportModalSize(m.width, m.height)
		m.importer.appendEvent(msg.ev, paddedModalContentWidth(boxW))
		if msg.ev.Stage == imp.StageError {
			return m, nil, true
		}
		if msg.ev.Stage == imp.StageDone {
			// Nhập thành công → Tiếp tục chuyển tiếp tự động: Tiếp tục sẽ kích hoạt Bộ định tuyến và gửi lệnh đầu tiên.
			// Thực hiện theo quy trình tiếp tục giống hệt như "Khởi động lại khôi phục dự án" (có thêm kết nối từ nhập phiên → tiếp tục).
			// Quá trình xử lý bootstrapMsg tiếp theo sẽ enterRunning() để chuyển sang trạng thái sáng tạo.
			return m, bootstrapRuntime(m.runtime), true
		}
		return m, listenImportEvent(msg.reqID, msg.ch), true
	case simEventMsg:
		if m.simulator == nil || msg.reqID != m.simulator.reqID {
			return m, nil, true
		}
		boxW, _ := reportModalSize(m.width, m.height)
		m.simulator.appendEvent(msg.ev, paddedModalContentWidth(boxW))
		if msg.terminal() {
			return m, nil, true
		}
		return m, listenSimulationEvent(msg.reqID, msg.ch), true
	case exportDoneMsg:
		if msg.err != nil {
			m.applyEvent(host.Event{
				Time: time.Now(), Category: "ERROR", Summary: "Xuất không thành công:" + msg.err.Error(), Level: "error",
			})
		} else if msg.result != nil {
			m.applyEvent(host.Event{
				Time: time.Now(), Category: "SYSTEM", Summary: formatExportSuccess(msg.result), Level: "success",
			})
		}
		m.refreshEventViewport()
		return m, nil, true
	case startResultMsg:
		next, cmd := m.handleStartResultMsg(msg)
		return next, cmd, true
	case cocreateDeltaMsg:
		if m.cocreate == nil || msg.reqID != m.cocreate.reqID {
			return m, nil, true
		}
		m.cocreate.applyDelta(msg.kind, msg.text)
		return m, listenCoCreateDelta(m.cocreate), true
	case cocreateDoneMsg:
		next, cmd := m.handleCoCreateDoneMsg(msg)
		return next, cmd, true
	case steerResultMsg:
		return m, tea.Batch(fetchSnapshot(m.runtime), listenDone(m.runtime)), true
	case continueResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.applyEvent(host.Event{
				Time: time.Now(), Category: "ERROR", Summary: msg.err.Error(), Level: "error",
			})
			m.refreshEventViewport()
			return m, tea.Batch(fetchSnapshot(m.runtime), m.textarea.Focus()), true
		}
		m.err = nil
		m.textarea.Placeholder = defaultSteerPlaceholder()
		return m, tea.Batch(fetchSnapshot(m.runtime), listenDone(m.runtime), m.textarea.Focus()), true
	case spinnerTickMsg:
		m.spinnerIdx = (m.spinnerIdx + 1) % len(spinnerFrames)
		if m.snapshot.IsRunning {
			// Làm mới hình ảnh của vòng quay hình sao/thanh trên cùng ở đây (350 mili giây)
			m.refreshEventViewport()
		}
		return m, tickSpinner(), true
	case toolSpinnerTickMsg:
		m.toolSpinnerIdx = (m.toolSpinnerIdx + 1) % len(toolSpinnerFrames)
		// Làm mới vòng quay cho các hàng luồng sự kiện "đang diễn ra" (150 mili giây, nhịp độ độc lập).
		// Khung spinner chỉ ảnh hưởng đến các dòng sự kiện đang chạy và kết quả hiển thị của các dòng đã hoàn thành là giống nhau theo từng byte;
		// Nếu không có sự kiện đang chạy, toàn bộ quá trình hiển thị lại sẽ vô nghĩa và bị bỏ qua.
		if m.snapshot.IsRunning && m.hasRunningEvent() {
			m.refreshEventViewport()
		}
		return m, tickToolSpinner(), true
	case cursorTickMsg:
		m.cursorIdx++
		if m.snapshot.IsRunning {
			// Nhấp nháy con trỏ yêu cầu hiển thị lại toàn bộ bảng phát trực tuyến (con trỏ ở cuối nội dung);
			// Nhân tiện, vết bẩn sẽ được làm sạch cùng nhau và dấu tích xả được thực hiện ngay lập tức, do đó không cần phải đánh răng lại.
			m.refreshStreamViewport()
			m.streamDirty = false
		}
		return m, tickCursor(), true
	case streamDeltaMsg:
		if len(m.streamRounds) == 0 {
			m.streamRounds = append(m.streamRounds, "")
		}
		m.streamRounds[len(m.streamRounds)-1] += string(msg)
		// Không làm mớiStreamViewport ngay lập tức, làm mới được hợp nhất bởi streamFlushTick 60fps.
		// Thời gian phát trực tuyến tốc độ cao của LLM là hàng chục mã thông báo mỗi giây và việc làm mới từng phân đoạn tương đương với việc hiển thị lại đầy đủ 32 phân đoạn hàng chục lần mỗi giây.
		m.streamDirty = true
		return m, listenStream(m.runtime), true
	case streamClearMsg:
		// Ranh giới tròn: loại bỏ vùng delta tích lũy trước để có thể căn chỉnh trực quan vòng mới
		if m.flushStreamIfDirty() && m.streamScroll {
			m.streamVP.GotoBottom()
		}
		if len(m.streamRounds) == 0 {
			m.streamRounds = append(m.streamRounds, "")
		} else if strings.TrimSpace(m.streamRounds[len(m.streamRounds)-1]) != "" {
			m.streamRounds = append(m.streamRounds, "")
		}
		m.trimStreamRounds()
		m.streamRound = len(m.streamRounds)
		m.refreshStreamViewport()
		if m.streamScroll {
			m.streamVP.GotoBottom()
		}
		return m, listenStream(m.runtime), true
	case streamFlushTickMsg:
		if m.flushStreamIfDirty() && m.streamScroll {
			m.streamVP.GotoBottom()
		}
		return m, tickStreamFlush(), true
	case quitResetMsg:
		m.quitPending = false
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleStartResultMsg(msg startResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		if m.mode != modeNew {
			m.applyEvent(host.Event{
				Time: time.Now(), Category: "ERROR", Summary: msg.err.Error(), Level: "error",
			})
			m.refreshEventViewport()
		}
		if m.cocreate != nil {
			m.cocreate.awaiting = false
			m.textarea.Placeholder = placeholderForCoCreate(m.cocreate)
			return m, tea.Batch(fetchSnapshot(m.runtime), m.textarea.Focus())
		}
		if m.mode == modeNew {
			m.textarea.Placeholder = placeholderForNewMode(m.startupMode)
			return m, tea.Batch(fetchSnapshot(m.runtime), m.textarea.Focus())
		}
		return m, fetchSnapshot(m.runtime)
	}

	if m.mode == modeNew {
		m.cocreate = nil
		enableMouse := m.enterRunning()
		m.resizeTextarea()
		m.textarea.Placeholder = defaultSteerPlaceholder()
		return m, tea.Batch(fetchSnapshot(m.runtime), m.textarea.Focus(), enableMouse)
	}

	return m, fetchSnapshot(m.runtime)
}

func (m Model) handleCoCreateDoneMsg(msg cocreateDoneMsg) (tea.Model, tea.Cmd) {
	if m.cocreate == nil || msg.reqID != m.cocreate.reqID {
		return m, nil
	}
	if msg.err != nil {
		m.err = msg.err
		m.cocreate.awaiting = false
		m.textarea.Placeholder = placeholderForCoCreate(m.cocreate)
		return m, m.textarea.Focus()
	}
	m.err = nil
	m.cocreate.apply(msg.reply)
	m.textarea.Placeholder = placeholderForCoCreate(m.cocreate)
	return m, m.textarea.Focus()
}

func (m Model) handleTextareaMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.refitTextareaHeight()
	m.updateCommandPalette()
	return m, cmd
}

// applyEvent áp dụng một sự kiện cho m.events:
// - Có ID và đã tồn tại → Cập nhật tại chỗ (hợp nhất các trường đã hoàn thành, giữ lại Thời gian/Tóm tắt đầu tiên)
// - sự kiện mới → nối thêm, đăng nhập vào eventIndex nếu cần
// - Thực hiện cắt ngắn trượt và xây dựng lại chỉ mục khi vượt quá maxEvents
func (m *Model) applyEvent(ev host.Event) {
	if ev.ID != "" {
		if idx, ok := m.eventIndex[ev.ID]; ok && idx >= 0 && idx < len(m.events) {
			existing := &m.events[idx]
			if !ev.FinishedAt.IsZero() {
				existing.FinishedAt = ev.FinishedAt
			}
			if ev.Duration > 0 {
				existing.Duration = ev.Duration
			}
			if ev.Failed {
				existing.Failed = true
			}
			if ev.Level != "" {
				existing.Level = ev.Level
			}
			// Cho phép ghi đè khi Tóm tắt không trống (trạng thái cuối có thể chứa thông tin bổ sung); nếu không, lần đầu tiên được giữ lại
			if ev.Summary != "" {
				existing.Summary = ev.Summary
			}
			return
		}
	}

	m.events = append(m.events, ev)
	if ev.ID != "" {
		m.eventIndex[ev.ID] = len(m.events) - 1
	}
	if len(m.events) > maxEvents {
		drop := len(m.events) - maxEvents
		m.events = m.events[drop:]
		m.rebuildEventIndex()
	}
}

// TrimStreamRounds cắt bớt luồngRounds thành phân đoạn maxStreamRounds; bất kỳ sự dư thừa nào sẽ bị loại bỏ ngay từ đầu.
// Thời điểm gọi: sau mỗi vòng phát trực tuyến mớiXóa, sau khi phát lại đã điền xong tất cả các mục lịch sử.
func (m *Model) trimStreamRounds() {
	if len(m.streamRounds) <= maxStreamRounds {
		return
	}
	drop := len(m.streamRounds) - maxStreamRounds
	m.streamRounds = m.streamRounds[drop:]
}

func (m *Model) rebuildEventIndex() {
	m.eventIndex = make(map[string]int, len(m.events))
	for i, e := range m.events {
		if e.ID != "" {
			m.eventIndex[e.ID] = i
		}
	}
}

func (m *Model) resetOutputPanels() {
	m.events = nil
	m.eventIndex = make(map[string]int)
	m.viewport.SetContent("")
	m.viewport.GotoTop()
	m.streamBuf.Reset()
	m.streamRounds = nil
	m.streamVP.SetContent("")
	m.streamVP.GotoTop()
	m.streamRound = 0
}

func (m *Model) applyRuntimeReplay(items []domain.RuntimeQueueItem) {
	for _, item := range items {
		switch item.Kind {
		case domain.RuntimeQueueUIEvent:
			// Luồng sự kiện không phát lại: chỉ có các sự kiện hoàn thành trong hàng đợi và Tác nhân/Độ sâu/Thời lượng/Cấp độ
			// Các trường cần thiết để hiển thị không được khôi phục khi phát lại và các hàng xuất hiện không đầy đủ. Tôi thà có một bảng trống hơn là dữ liệu đầy một nửa.
			continue
		case domain.RuntimeQueueStreamClear:
			if len(m.streamRounds) == 0 {
				m.streamRounds = append(m.streamRounds, "")
			} else if strings.TrimSpace(m.streamRounds[len(m.streamRounds)-1]) != "" {
				m.streamRounds = append(m.streamRounds, "")
			}
		case domain.RuntimeQueueStreamDelta:
			text := host.ReplayDeltaText(item)
			if text == "" {
				continue
			}
			if len(m.streamRounds) == 0 {
				m.streamRounds = append(m.streamRounds, "")
			}
			m.streamRounds[len(m.streamRounds)-1] += text
		}
	}
	m.trimStreamRounds()
	m.streamRound = len(m.streamRounds)
	m.refreshEventViewport()
	m.refreshStreamViewport()
}
