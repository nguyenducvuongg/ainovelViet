package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nguyenducvuongg/ainovelViet/internal/diag"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/entry/startup"
	"github.com/nguyenducvuongg/ainovelViet/internal/host"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// Loại tin nhắn
type (
	eventMsg       host.Event
	snapshotMsg    host.UISnapshot
	doneMsg        struct{ complete bool } // Complete=true hoàn thành cuốn sách, sai dừng khi có lỗi
	abortResultMsg struct{ stopped bool }
	bootstrapMsg   struct {
		replay  []domain.RuntimeQueueItem
		resumed bool
		err     error
	}
	reportLoadedMsg struct {
		reqID      int
		report     diag.Report
		exportPath string // Đường dẫn tuyệt đối đến tệp chẩn đoán giải mẫn cảm; trống = xuất không thành công
		finishedAt time.Time
	}
	askUserMsg       askUserRequest
	startResultMsg   struct{ err error }
	cocreateDeltaMsg struct {
		reqID int
		kind  string // host.CoCreateProgressThinking | host.CoCreateProgressReply
		text  string
	}
	// cocreateStreamItem là tải trọng nội bộ deltaCh cung cấp kiểu phát trực tuyến cho TUI cùng với văn bản tích lũy.
	cocreateStreamItem struct {
		kind string
		text string
	}
	cocreateDoneMsg struct {
		reqID int
		reply host.CoCreateReply
		err   error
	}
	steerResultMsg     struct{}
	continueResultMsg  struct{ err error }
	spinnerTickMsg     time.Time
	toolSpinnerTickMsg time.Time // Đánh dấu độc lập của công cụ quay vòng sự kiện (nhanh hơn, không phụ thuộc vào thanh trên cùng/sao)
	cursorTickMsg      time.Time // Truyền con trỏ đánh dấu độc lập
	streamDeltaMsg     string    // Tăng mã thông báo phát trực tuyến
	streamClearMsg     struct{}  // Xóa bộ đệm phát trực tuyến (bắt đầu tin nhắn mới)
	streamFlushTickMsg struct{}  // Bảng điều khiển phát trực tuyến làm mới điều chỉnh tốc độ 60 khung hình/giây (hợp nhất delta cấp mã thông báo)
	quitResetMsg       struct{}  // Đặt lại thời gian chờ Ctrl + C kép
)

// --- Hàm cmd ---

func listenEvents(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-rt.Events()
		if !ok {
			return nil
		}
		return eventMsg(ev)
	}
}

func listenDone(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-rt.Done()
		if !ok {
			return nil
		}
		snap := rt.Snapshot()
		return doneMsg{complete: snap.Phase == "complete"}
	}
}

func tickSnapshot(rt *host.Host) tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return snapshotMsg(rt.Snapshot())
	})
}

func fetchSnapshot(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		return snapshotMsg(rt.Snapshot())
	}
}

func bootstrapRuntime(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		replay, err := rt.ReplayQueue(0)
		if err != nil {
			return bootstrapMsg{err: err}
		}
		label, err := rt.Resume()
		if err != nil {
			return bootstrapMsg{replay: replay, err: err}
		}
		if label == "" && len(replay) == 0 {
			return nil
		}
		return bootstrapMsg{replay: replay, resumed: label != ""}
	}
}

func startRuntime(rt *host.Host, plan startup.Plan) tea.Cmd {
	return func() tea.Msg {
		err := rt.StartPrepared(plan.StartPrompt)
		return startResultMsg{err: err}
	}
}

func runCoCreate(rt *host.Host, state *cocreateState) tea.Cmd {
	history := state.session.History()
	ctx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	state.deltaCh = make(chan cocreateStreamItem, 64)
	state.doneCh = make(chan cocreateDoneMsg, 1)
	// Đồng sáng tạo theo giai đoạn với tóm tắt trạng thái câu chuyện, đầu ra “tóm tắt hướng tiếp theo”; khởi động nguội để làm rõ các yêu cầu từ đầu. Cả hai chữ ký đều trùng khớp.
	stream := rt.CoCreateStream
	if state.stage {
		stream = rt.StageCoCreateStream
	}
	start := func() tea.Msg {
		go func() {
			reply, err := stream(ctx, history, func(kind, text string) {
				select {
				case state.deltaCh <- cocreateStreamItem{kind: kind, text: text}:
				default:
				}
			})
			state.doneCh <- cocreateDoneMsg{reply: reply, err: err}
			close(state.deltaCh)
			close(state.doneCh)
		}()
		return nil
	}
	return tea.Batch(start, listenCoCreateDelta(state), listenCoCreateDone(state))
}

func listenCoCreateDelta(state *cocreateState) tea.Cmd {
	if state == nil || state.deltaCh == nil {
		return nil
	}
	// Lấy tham chiếu cục bộ của kênh: tránh trạng thái tiếp theo.deltaCh được chỉ định lại
	// Việc đóng nghe cũ đọc sai kênh mới (mặc dù quy trình hiện tại không được kích hoạt, nhưng không nên để nó như một cái bẫy bảo trì).
	reqID := state.reqID
	ch := state.deltaCh
	return func() tea.Msg {
		item, ok := <-ch
		if !ok {
			return nil
		}
		return cocreateDeltaMsg{reqID: reqID, kind: item.kind, text: item.text}
	}
}

func listenCoCreateDone(state *cocreateState) tea.Cmd {
	if state == nil || state.doneCh == nil {
		return nil
	}
	reqID := state.reqID
	ch := state.doneCh
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return nil
		}
		result.reqID = reqID
		return result
	}
}

func steerRuntime(rt *host.Host, text string) tea.Cmd {
	return func() tea.Msg {
		rt.Steer(text)
		return steerResultMsg{}
	}
}

func continueRuntime(rt *host.Host, text string) tea.Cmd {
	return func() tea.Msg {
		err := rt.Continue(text)
		return continueResultMsg{err: err}
	}
}

// sơ yếu lý lịchFromCoCreate chèn bản tóm tắt hướng tiếp theo của đầu ra đồng tạo giai đoạn và tiếp tục quá trình tạo.
// Tái sử dụng continueResultMsg: Nếu thành công, listenDone sẽ được sử dụng để tiếp tục chạy. Nếu thất bại, một lỗi sẽ được lặp lại.
func resumeFromCoCreate(rt *host.Host, draft string) tea.Cmd {
	return func() tea.Msg {
		err := rt.ResumeFromCoCreate(draft)
		return continueResultMsg{err: err}
	}
}

// cancelCoCreate từ bỏ giai đoạn đồng sáng tạo: xóa dấu chiếm đóng và tiếp tục tạm dừng. Các sự kiện được truyền ngược lại qua kênh sự kiện mà không trả lại tin nhắn.
func cancelCoCreate(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		rt.CancelCoCreate()
		return nil
	}
}

func abortRuntime(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		return abortResultMsg{stopped: rt.Abort()}
	}
}

func loadReport(dir string, reqID int) tea.Cmd {
	return func() tea.Msg {
		s := store.NewStore(dir)
		// Chẩn đoán = chẩn đoán tạo + phát hiện thời gian chạy, thời gian chạy Tìm cũng báo cáo trên màn hình.
		rep, rc := diag.Diagnose(s)
		// Sử dụng lại Rep+rc để ghi các tệp chẩn đoán giải mẫn cảm (lỗi xuất sẽ không ảnh hưởng đến báo cáo trên màn hình).
		exportPath, _ := diag.WriteExport(s, rep, rc)
		return reportLoadedMsg{
			reqID:      reqID,
			report:     rep,
			exportPath: exportPath,
			finishedAt: time.Now(),
		}
	}
}

func tickSpinner() tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

// tickToolSpinner Công cụ quay vòng điều khiển dòng "đang tiến hành" của luồng sự kiện. Độc lập với tickSpinner, nhanh hơn (150ms).
func tickToolSpinner() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return toolSpinnerTickMsg(t)
	})
}

func tickCursor() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg {
		return cursorTickMsg(t)
	})
}

// tickStreamFlush thúc đẩy quá trình làm mới điều tiết của bảng điều khiển phát trực tuyến. streamingDelta không còn hiển thị lại từng mã thông báo ngay lập tức nữa,
// Nhưng đánh dấu bẩn; đánh dấu này sẽ kiểm tra, hợp nhất và làm mới sau mỗi 16 mili giây (~60 khung hình/giây) và phát trực tuyến tốc độ cao LLM
// "Hàng chục lần hiển thị lại đầy đủ mỗi giây" trong khoảng thời gian này đã đẩy lùi giới hạn trên là 60 lần.
func tickStreamFlush() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
		return streamFlushTickMsg{}
	})
}

func listenStream(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		delta, ok := <-rt.Stream()
		if !ok {
			return nil
		}
		// Sentinel được gửi đi dưới dạng luồngClearMsg, được đảm bảo ở cùng kênh với delta thông thường bằng cách nhấn phát ra.
		// Đến TUI theo trình tự. Khi có hai kênh, không có thứ tự giữa ClearCh và streamCh, ✻ tiêu đề thường
		// Nó đã bị chèn nhầm vào cuối đoạn suy nghĩ trước đó.
		if delta == host.StreamClearSentinel {
			return streamClearMsg{}
		}
		return streamDeltaMsg(delta)
	}
}

func listenAskUser(bridge *askUserBridge) tea.Cmd {
	return func() tea.Msg {
		req, ok := <-bridge.requests
		if !ok {
			return nil
		}
		return askUserMsg(req)
	}
}
