package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// errorKind classifies a runtime error into a stable, short label for log
// filtering and alert routing. Returns "" when no special tag applies.
//
// err is the live error chain (may be nil after JSON serialization); msg is
// the rendered string fallback used when the chain has been flattened
// (e.g. inside sub-agent JSON results).
func errorKind(err error, msg string) string {
	if err != nil && errors.Is(err, agentcore.ErrProviderStreamIdle) {
		return "stream_idle"
	}
	if msg != "" && agentcore.IsStreamIdleMessage(msg) {
		return "stream_idle"
	}
	return ""
}

// Bộ đếm ID sự kiện tăng đơn điệu; khớp với dấu thời gian để tạo ID ổn định.
var eventIDCounter uint64

func nextEventID() string {
	return fmt.Sprintf("e%d", atomic.AddUint64(&eventIDCounter, 1))
}

// activeCall ghi lại ID, thời gian bắt đầu và tóm tắt cuộc gọi đang diễn ra (TOOL/DISPATCH).
// bản tóm tắt được điền lại vào Sự kiện kết thúc khi sự kiện được hoàn thành để đảm bảo rằng việc phát lại (hàng đợi thời gian chạy) có thể khôi phục nội dung hàng.
type activeCall struct {
	id      string
	start   time.Time
	summary string
	depth   int
}

// Người quan sát đăng ký luồng sự kiện điều phối viên và chiếu nó tới kênh đầu ra của Máy chủ.
// Nó là một người quan sát thuần túy và không tham gia vào bất kỳ quyết định kiểm soát nào.
type observer struct {
	unsub   func()
	emitEv  func(Event)
	emitD   func(string)
	emitC   func()
	store   *storepkg.Store // Được sử dụng để duy trì hàng đợi thời gian chạy (tiêu thụ ReplayQueue)
	agents  map[string]*agentState
	agentMu sync.Mutex

	// Việc hủy bỏ được Máy chủ thiết lập tại mục nhập Abort()/Close() và bị xóa bởi Bắt đầu/Tiếp tục/Tiếp tục.
	// Tất cả các sự kiện lỗi bắt nguồn từ việc hủy ngữ cảnh đều bị chặn trong khi thiết lập (cả hai đều như người dùng mong đợi và để tránh xung đột với
	// Sự kiện "Tạm dừng hướng dẫn sử dụng" được lặp lại). Các trường hợp ngoại lệ thực sự (không hủy) vẫn được báo cáo như thường lệ.
	aborting atomic.Bool

	streamThinking        bool
	lastThinkingByAgent   map[string]string          // tác nhân → văn bản suy nghĩ tích lũy gần đây (được sử dụng để trích xuất delta delta)
	dispatchStarts        map[string]*activeCall     // đại lý được phái đi → cuộc gọi DISPATCH đang diễn ra
	currentDispatchTarget string                     // Tên của tác nhân phụ hiện đang thực thi (Args có thể trống đối với handToolEnd)
	toolStarts            map[string]*activeCall     // đại lý → cuộc gọi TOOL đang diễn ra
	streamExtractors      map[string]*agentExtractor // tác nhân → Trình trích xuất nội dung cho các tham số JSON được gọi bởi công cụ hiện tại
	streamArgPrefixes     map[string]string          // tác nhân/công cụ → tiền tố luồng tham số, được sử dụng để xác định trước các thẻ nhẹ
	streamArgLabels       map[string]string          // tác nhân/công cụ → tên hiển thị đã được nhận dạng trước từ luồng tham số
	streamHasContent      bool                       // StreamRound hiện tại có nội dung đầu ra hay không (để xác định xem có cần tách đoạn hay không)
	streamLastByte        byte                       // Byte cuối cùng của đầu ra được phát gần đây nhất (được sử dụng để hoàn thành chính xác các dòng mới)
}

// AgentExtractor ghi lại tên công cụ và phiên bản trình trích xuất hiện đang được tác nhân trích xuất.
// Tên công cụ được sử dụng để phát hiện "cuộc gọi công cụ mới đã bắt đầu" nhằm tránh bộ đệm bị ô nhiễm bởi phần còn lại của vòng trước.
type agentExtractor struct {
	tool       string
	ext        *jsonFieldExtractor
	emittedAny bool // Liệu trình trích xuất này đã tạo ra nội dung hay chưa; nó được sử dụng để thêm phân tách đoạn văn trước khi xuất ra lần đầu tiên.
}

type agentState struct {
	name    string
	state   string
	tool    string
	summary string
	turn    int
	context AgentContextSnapshot
	updated time.Time
}

func newObserver(coordinator *agentcore.Agent, s *storepkg.Store, emitEv func(Event), emitD func(string), emitC func()) *observer {
	o := &observer{
		emitEv:              emitEv,
		emitD:               emitD,
		emitC:               emitC,
		store:               s,
		agents:              make(map[string]*agentState),
		lastThinkingByAgent: make(map[string]string),
		dispatchStarts:      make(map[string]*activeCall),
		toolStarts:          make(map[string]*activeCall),
		streamExtractors:    make(map[string]*agentExtractor),
		streamArgPrefixes:   make(map[string]string),
		streamArgLabels:     make(map[string]string),
	}
	o.unsub = coordinator.Subscribe(o.handle)
	return o
}

func (o *observer) finalize() {
	o.agentMu.Lock()
	defer o.agentMu.Unlock()
	for _, a := range o.agents {
		a.state = "idle"
		a.tool = ""
	}
}

// setAborting được Máy chủ gọi khi chuyển đổi vòng đời như Hủy bỏ/Đóng/Bắt đầu và được điều khiển bởi
// Liệu các sự kiện dẫn xuất "ngữ cảnh đã bị hủy" có cần được loại bỏ hay không (để tránh trùng lặp với "tạm dừng thủ công người dùng").
func (o *observer) setAborting(v bool) { o.aborting.Store(v) }

// isCancellationNoise xác định xem lỗi có phải là nhiễu do hủy bỏ hay không.
// Trả về true chỉ có ý nghĩa khi Máy chủ ở trạng thái hủy bỏ - trong khoảng thời gian không hủy bỏ
// bối cảnh. Đã hủy có thể phản ánh một vấn đề thực sự (chẳng hạn như ctx bên ngoài bị hủy) và vẫn phải được báo cáo.
func (o *observer) isCancellationNoise(err error, msg string) bool {
	if !o.aborting.Load() {
		return false
	}
	if err != nil && errors.Is(err, context.Canceled) {
		return true
	}
	return strings.Contains(strings.ToLower(msg), "context canceled")
}

// phát raAndLog được sử dụng để gọi trạng thái "bắt đầu" của các sự kiện trong lớp: được gửi tới TUI nhưng không được ghi vào hàng đợi thời gian chạy.
// Tránh trùng lặp "bắt đầu một dòng và hoàn thành một dòng khác" trong khi phát lại. khẩu hiệu được ghi lại một cách thống nhất bởi Host.emitEvent.
func (o *observer) emitAndLog(ev Event) {
	o.emitEv(ev)
}

// PersistEvent ghi các sự kiện vào hàng đợi thời gian chạy (slog được ghi lại một cách thống nhất bởi Host.emitEvent).
func (o *observer) persistEvent(ev Event) {
	if o.store == nil || o.store.Runtime == nil {
		return
	}
	priority := domain.RuntimePriorityBackground
	switch ev.Category {
	case "SYSTEM", "ERROR":
		priority = domain.RuntimePriorityControl
	}
	_, _ = o.store.Runtime.AppendQueue(domain.RuntimeQueueItem{
		Time:     ev.Time,
		Kind:     domain.RuntimeQueueUIEvent,
		Priority: priority,
		Category: ev.Category,
		Summary:  ev.Summary,
		Payload:  ev,
	})
}

func (o *observer) handle(ev agentcore.Event) {
	switch ev.Type {
	case agentcore.EventToolExecStart:
		o.handleToolStart(ev)
	case agentcore.EventToolExecUpdate:
		o.handleToolUpdate(ev)
	case agentcore.EventToolExecEnd:
		o.handleToolEnd(ev)
	case agentcore.EventMessageUpdate:
		o.handleMessageUpdate(ev)
	case agentcore.EventMessageEnd:
		o.streamClear()
	case agentcore.EventTurnStart:
		if ev.Progress != nil && ev.Progress.Kind == agentcore.ProgressTurnCounter {
			o.updateAgent(ev.Progress.Agent, func(a *agentState) {
				a.turn = ev.Progress.Turn
			})
		}
	case agentcore.EventRetry:
		if ev.RetryInfo != nil {
			msg := ""
			if ev.RetryInfo.Err != nil {
				msg = ev.RetryInfo.Err.Error()
			}
			prefix := fmt.Sprintf("Thử lại (%d/%d): ", ev.RetryInfo.Attempt, ev.RetryInfo.MaxRetries)
			retryEv := Event{
				Time:     time.Now(),
				Category: "SYSTEM",
				Summary:  prefix + truncate(msg, 80),
				Detail:   prefix + msg,
				Kind:     errorKind(ev.RetryInfo.Err, msg),
				Level:    "warn",
			}
			o.emitEv(retryEv)
			o.persistEvent(retryEv)
		}
	case agentcore.EventError:
		if ev.Err != nil {
			fullMsg := ev.Err.Error()
			if o.isCancellationNoise(ev.Err, fullMsg) {
				// lỗi hủy ctx bắt nguồn từ việc hủy bỏ do người dùng thực hiện; đã xảy ra sự kiện "tạm dừng hướng dẫn sử dụng" và màn hình sẽ không được làm mới lại.
				o.flushActiveCalls(true)
				slog.Debug("suppressed cancel-derived error", "module", "agent", "msg", fullMsg)
				return
			}
			o.flushActiveCalls(true)
			errEv := Event{
				Time:     time.Now(),
				Category: "ERROR",
				Summary:  truncate(fullMsg, 120),
				Detail:   fullMsg,
				Kind:     errorKind(ev.Err, fullMsg),
				Level:    "error",
			}
			o.emitEv(errEv)
			o.persistEvent(errEv)
		}
	}
}

func (o *observer) handleMessageUpdate(ev agentcore.Event) {
	if ev.Delta == "" {
		return
	}
	if ev.DeltaKind == agentcore.DeltaToolCall {
		o.handleCoordinatorToolDelta(ev)
		return
	}
	o.emitStreamDelta(ev.Delta, ev.DeltaKind == agentcore.DeltaThinking)
}

func (o *observer) handleToolStart(ev agentcore.Event) {
	if ev.Tool == "" {
		return
	}
	agent := agentFromEvent(ev)

	// cuộc gọi đại lý phụ → sự kiện DISPATCH (đang diễn ra)
	if ev.Tool == "subagent" {
		sub := parseSubagentArgs(ev.Args)
		target := sub.agent
		if target == "" {
			target = "subagent"
		}
		dispatchSummary := dispatchSummary(target, sub.task)
		o.updateAgent(agent, func(a *agentState) {
			a.state = "working"
			a.tool = ev.Tool
			a.summary = fmt.Sprintf("%s → %s", agent, dispatchSummary)
		})
		o.currentDispatchTarget = target
		if call, ok := o.dispatchStarts["subagent"]; ok {
			delete(o.dispatchStarts, "subagent")
			o.dispatchStarts[target] = call
			o.updateDispatchSummary(target, dispatchSummary)
			return
		}
		id := nextEventID()
		o.dispatchStarts[target] = &activeCall{id: id, start: time.Now(), summary: dispatchSummary}
		o.emitAndLog(Event{
			ID:       id,
			Time:     time.Now(),
			Category: "DISPATCH",
			Agent:    agent,
			Summary:  dispatchSummary,
			Level:    "info",
		})
		return
	}

	// công cụ riêng của điều phối viên (đang tiến hành)
	toolName := displayToolName(ev.Tool, ev.Args)
	if _, ok := o.toolStarts[agent]; ok {
		o.updateToolCallSummary(agent, ev.Tool, toolName)
		return
	}
	o.updateAgent(agent, func(a *agentState) {
		a.state = "working"
		a.tool = ev.Tool
		a.summary = fmt.Sprintf("%s → %s", agent, toolName)
	})
	id := nextEventID()
	o.toolStarts[agent] = &activeCall{id: id, start: time.Now(), summary: toolName}
	o.emitAndLog(Event{
		ID:       id,
		Time:     time.Now(),
		Category: "TOOL",
		Agent:    agent,
		Summary:  toolName,
		Level:    "info",
	})
	o.emitFallbackStreamHeader(ev.Tool)
}

func (o *observer) handleToolUpdate(ev agentcore.Event) {
	if ev.Progress == nil {
		return
	}
	switch ev.Progress.Kind {
	case agentcore.ProgressToolDelta:
		if ev.Progress.Delta != "" {
			o.handleSubagentDelta(ev.Progress)
		}
	case agentcore.ProgressToolStart:
		// Công cụ gọi bên trong các tác nhân phụ (ví dụ: người viết → bản nháp_chapter).
		// LƯU Ý: Các dòng CÔNG CỤ có thể đã được handSubagentDelta phát ra sớm trong giai đoạn nhận dạng phát trực tuyến.
		// Tại đây: Nếu nó đã được gửi → chỉ cập nhật tóm tắt (args hiện đã hoàn tất và có thể hiển thị "tool (Chương N)"); nếu không, nó sẽ được gửi bình thường.
		if ev.Progress.Agent == "" || ev.Progress.Tool == "" {
			break
		}
		toolName := displayToolName(ev.Progress.Tool, ev.Progress.Args)
		if _, ok := o.toolStarts[ev.Progress.Agent]; ok {
			o.updateToolCallSummary(ev.Progress.Agent, ev.Progress.Tool, toolName)
			o.updateAgent(ev.Progress.Agent, func(a *agentState) {
				a.state = "working"
				a.tool = ev.Progress.Tool
				a.summary = fmt.Sprintf("%s → %s", ev.Progress.Agent, toolName)
			})
			break
		}
		// Không được gửi trước → quy trình bình thường
		// (Các mô hình có đối số công cụ không phát trực tuyến sẽ không kích hoạt EnsureSubagentToolStarted,
		// Tiêu đề dự phòng phải được thêm một lần trên đường dẫn này, nếu không read_chapter sẽ
		// Không có tiêu đề ✻ trên bảng điều khiển luồng công cụ mà không có trình trích xuất, vì vậy hãy suy nghĩ về nó một lúc. )
		id := nextEventID()
		o.toolStarts[ev.Progress.Agent] = &activeCall{id: id, start: time.Now(), summary: toolName, depth: 1}
		o.emitAndLog(Event{
			ID:       id,
			Time:     time.Now(),
			Category: "TOOL",
			Agent:    ev.Progress.Agent,
			Summary:  toolName,
			Level:    "info",
			Depth:    1,
		})
		o.updateAgent(ev.Progress.Agent, func(a *agentState) {
			a.state = "working"
			a.tool = ev.Progress.Tool
			a.summary = fmt.Sprintf("%s → %s", ev.Progress.Agent, toolName)
		})
		o.emitFallbackStreamHeader(ev.Progress.Tool)
	case agentcore.ProgressToolEnd:
		delete(o.streamExtractors, ev.Progress.Agent)
		if ev.Progress.Agent == "" {
			return
		}
		call, ok := o.toolStarts[ev.Progress.Agent]
		if !ok {
			return
		}
		delete(o.toolStarts, ev.Progress.Agent)
		// Sự kiện cập nhật ID giống nhau: TUI định vị hàng CÔNG CỤ ban đầu theo ID và chèn lấp FinishedAt / Duration.
		// Tóm tắt / Độ sâu cũng được đưa vào để đảm bảo rằng dòng hoàn chỉnh có thể được khôi phục trong quá trình phát lại hàng đợi thời gian chạy.
		finishEv := Event{
			ID:         call.id,
			Time:       call.start,
			FinishedAt: time.Now(),
			Category:   "TOOL",
			Agent:      ev.Progress.Agent,
			Summary:    call.summary,
			Level:      "info",
			Depth:      call.depth,
			Duration:   time.Since(call.start),
		}
		o.emitEv(finishEv)
		o.persistEvent(finishEv)
	case agentcore.ProgressThinking:
		o.handleThinkingProgress(ev)
	case agentcore.ProgressRetry:
		prefix := fmt.Sprintf("Thử lại (%d/%d): ", ev.Progress.Attempt, ev.Progress.MaxRetries)
		retryEv := Event{
			Time:     time.Now(),
			Category: "SYSTEM",
			Agent:    ev.Progress.Agent,
			Summary:  prefix + truncate(ev.Progress.Message, 80),
			Detail:   prefix + ev.Progress.Message,
			Kind:     errorKind(nil, ev.Progress.Message),
			Level:    "warn",
			Depth:    1,
		}
		o.emitEv(retryEv)
		o.persistEvent(retryEv)
	case agentcore.ProgressToolError:
		delete(o.streamExtractors, ev.Progress.Agent)
		msg := ev.Progress.Message
		if msg == "" {
			msg = "unknown error"
		}
		// Nếu có một dòng CÔNG CỤ đang được xử lý, hãy đánh dấu nó là không thành công; nếu không thì nối thêm dòng LỖI một cách độc lập.
		if call, ok := o.toolStarts[ev.Progress.Agent]; ok {
			delete(o.toolStarts, ev.Progress.Agent)
			finishEv := Event{
				ID:         call.id,
				Time:       call.start,
				FinishedAt: time.Now(),
				Failed:     true,
				Category:   "TOOL",
				Agent:      ev.Progress.Agent,
				Summary:    call.summary,
				Level:      "error",
				Depth:      call.depth,
				Duration:   time.Since(call.start),
			}
			o.emitEv(finishEv)
			o.persistEvent(finishEv)
		}
		// Đính kèm dòng chi tiết LỖI (bổ sung thông tin lỗi để hỗ trợ khắc phục sự cố)
		errEv := Event{
			Time:     time.Now(),
			Category: "ERROR",
			Agent:    ev.Progress.Agent,
			Summary:  fmt.Sprintf("Lỗi %s: %s", ev.Progress.Tool, truncate(msg, 100)),
			Detail:   fmt.Sprintf("Lỗi %s: %s", ev.Progress.Tool, msg),
			Kind:     errorKind(nil, msg),
			Level:    "error",
			Depth:    1,
		}
		o.emitEv(errEv)
		o.persistEvent(errEv)
	case agentcore.ProgressContext:
		o.handleContextProgress(ev)
	}
}

// Các tham số lệnh gọi công cụ và văn bản của phần tử con của handSubagentDelta shunt:
// - DeltaText chảy trực tiếp dưới dạng đánh dấu
// - DeltaToolCall chỉ trích xuất các trường từ các công cụ có nội dung dài đã biết (chẳng hạn như Draft_chapter.content); tất cả tham số JSON của các công cụ khác đều bị loại bỏ.
func (o *observer) handleSubagentDelta(p *agentcore.ProgressPayload) {
	if p.DeltaKind != agentcore.DeltaToolCall {
		o.emitStreamDelta(p.Delta, false)
		return
	}
	if p.Tool == "" {
		return // Tên công cụ chưa sẵn sàng, hãy thử lại với delta tiếp theo
	}

	// Khi tên công cụ được nhận dạng bằng phương pháp phát trực tuyến, sự kiện đang xử lý TOOL sẽ được gửi trước, cho phép công cụ quay vòng bao trùm toàn bộ giai đoạn tạo LLM.
	// (Nếu không thì thông tin "đang tiến hành" của các công cụ như Draft_chapter chỉ được hiển thị trong khoảng chục mili giây của quá trình Thực thi thực).
	// Khi ProgressToolStart thực sự đến, người ta nhận ra rằng toolStarts đã có bản ghi và sẽ chỉ hoàn thành phần tóm tắt.
	o.ensureSubagentToolStarted(p.Agent, p.Tool)
	o.updateToolCallSummaryFromDelta(p.Agent, p.Tool, p.Delta)

	cur, ok := o.streamExtractors[p.Agent]
	// Trailing delta vẫn có thể được nhận sau khi lệnh gọi công cụ tương tự đã bị đóng (lần truy cập cấp cao nhất }):
	// Một số nhà cung cấp (thử nghiệm thực tế deepseek-v4-flash) sẽ chia một đối số thành nhiều phần.
	// Đoạn cuối cùng được theo sau bởi một ký tự trống hoặc lặp lại sau `}`. Lúc này nếu nhấn "Tool Name Match +
	// Xong có nghĩa là xử lý "xây dựng lại", trình trích xuất mới sẽ phát ra một lần nữa ✻ tiêu đề và mã thông báo đoạn đuôi
	// Được phân tích cú pháp dưới dạng đối số mới. Những vùng đồng bằng này là những cái đuôi dư thừa và có thể bị loại bỏ.
	if ok && cur.tool == p.Tool && cur.ext.Done() {
		return
	}
	// Tên công cụ đã thay đổi hoặc chưa được xây dựng: Tạo một cái mới.
	if !ok || cur.tool != p.Tool {
		ext := newToolExtractor(p.Tool)
		if ext == nil {
			delete(o.streamExtractors, p.Agent)
			return
		}
		cur = &agentExtractor{tool: p.Tool, ext: ext}
		o.streamExtractors[p.Agent] = cur
	}
	if emitted := cur.ext.Feed(p.Delta); emitted != "" {
		if !cur.emittedAny {
			cur.emittedAny = true
			// StreamClear làm cho tiêu đề ✻ của trình trích xuất rơi vào điểm bắt đầu của vòng mới, khớp với
			// HasPrefix("✻") của renderStreamContent kiểm tra việc làm nổi bật renderAgentBlock
			// Con đường; sử dụng EnsureStreamParagraphBreak để chỉ chèn dòng trống mà không mở vòng, ✻ vẫn sẽ
			// Suy nghĩ/văn bản trước đó được bao bọc và rơi vào renderChapterBlock và được vẽ bằng màu mặc định.
			o.streamClear()
			// StreamClear xóa luồng một cách phòng thủ. Cur hiện tại vẫn cần tiếp tục cho ăn
			// Khi công cụ này gọi delta tiếp theo, nó phải được đăng ký lại ngay lập tức; mặt khác, đồng bằng tiếp theo
			// Khi nó xuất hiện, một trình trích xuất mới sẽ được tạo và quá trình phân tích cú pháp sẽ bắt đầu từ giữa các đối số (tại `{` của đối tượng lồng nhau
			// Trước khi vào psBeforeKey), đặt dòng thời gian_events.time / foreshadow_updates.id
			// Khi được sử dụng làm trường cấp cao nhất, tiêu đề ✻ xuất hiện liên tục trên TUI.
			o.streamExtractors[p.Agent] = cur
		}
		o.emitStreamDelta(emitted, false)
	}
}

func (o *observer) handleCoordinatorToolDelta(ev agentcore.Event) {
	msg, ok := ev.Message.(agentcore.Message)
	if !ok {
		return
	}
	call, ok := latestToolCall(msg)
	if !ok || call.Name == "" {
		return
	}
	if call.Name == "subagent" {
		o.ensureCoordinatorDispatchStarted(call)
		o.updateCoordinatorDispatchSummaryFromDelta(ev.Delta)
		return
	}
	o.ensureCoordinatorToolStarted(call.Name)
	o.updateToolCallSummaryFromDelta("coordinator", call.Name, ev.Delta)
}

func latestToolCall(msg agentcore.Message) (agentcore.ToolCall, bool) {
	calls := msg.ToolCalls()
	if len(calls) == 0 {
		return agentcore.ToolCall{}, false
	}
	return calls[len(calls)-1], true
}

func (o *observer) ensureCoordinatorToolStarted(tool string) {
	const agent = "coordinator"
	if tool == "" {
		return
	}
	if _, ok := o.toolStarts[agent]; ok {
		return
	}
	o.resetStreamArgLabel(agent, tool)
	id := nextEventID()
	o.toolStarts[agent] = &activeCall{id: id, start: time.Now(), summary: tool}
	o.updateAgent(agent, func(a *agentState) {
		a.state = "working"
		a.tool = tool
		a.summary = fmt.Sprintf("%s → %s", agent, tool)
	})
	o.emitAndLog(Event{
		ID:       id,
		Time:     time.Now(),
		Category: "TOOL",
		Agent:    agent,
		Summary:  tool,
		Level:    "info",
	})
	o.emitFallbackStreamHeader(tool)
}

func (o *observer) ensureCoordinatorDispatchStarted(call agentcore.ToolCall) {
	if _, ok := o.dispatchStarts["subagent"]; ok {
		return
	}
	o.resetStreamArgLabel("coordinator", call.Name)
	id := nextEventID()
	o.dispatchStarts["subagent"] = &activeCall{id: id, start: time.Now(), summary: "subagent"}
	o.currentDispatchTarget = "subagent"
	o.updateAgent("coordinator", func(a *agentState) {
		a.state = "working"
		a.tool = call.Name
		a.summary = "coordinator → subagent"
	})
	o.emitAndLog(Event{
		ID:       id,
		Time:     time.Now(),
		Category: "DISPATCH",
		Agent:    "coordinator",
		Summary:  "subagent",
		Level:    "info",
	})
}

func (o *observer) updateCoordinatorDispatchSummaryFromDelta(delta string) {
	const key = "subagent"
	prefix := o.streamArgPrefixes[streamArgKey("coordinator", key)] + delta
	if len(prefix) > 1024 {
		prefix = prefix[:1024]
	}
	o.streamArgPrefixes[streamArgKey("coordinator", key)] = prefix

	agent := firstJSONStringField(prefix, "agent")
	if agent == "" {
		return
	}
	task := firstJSONStringField(prefix, "task")
	summary := dispatchSummary(agent, task)
	labelKey := streamArgKey("coordinator", key)
	if o.streamArgLabels[labelKey] == summary {
		return
	}
	o.streamArgLabels[labelKey] = summary
	o.updateDispatchSummary("subagent", summary)
}

func dispatchSummary(agent, task string) string {
	if agent == "" {
		agent = "subagent"
	}
	if task == "" {
		return agent
	}
	firstLine := strings.TrimSpace(strings.SplitN(task, "\n", 2)[0])
	if firstLine == "" {
		return agent
	}
	return agent + "（" + truncate(firstLine, 30) + "）"
}

func (o *observer) updateToolCallSummary(agent, tool, summary string) {
	if agent == "" || summary == "" {
		return
	}
	call, ok := o.toolStarts[agent]
	if !ok || call.summary == summary {
		return
	}
	call.summary = summary
	o.emitEv(Event{
		ID:       call.id,
		Time:     call.start,
		Category: "TOOL",
		Agent:    agent,
		Summary:  summary,
		Level:    "info",
		Depth:    call.depth,
	})
	o.updateAgent(agent, func(a *agentState) {
		a.state = "working"
		a.tool = tool
		a.summary = fmt.Sprintf("%s → %s", agent, summary)
	})
}

func (o *observer) updateDispatchSummary(target, summary string) {
	if target == "" || summary == "" {
		return
	}
	call, ok := o.dispatchStarts[target]
	if !ok || call.summary == summary {
		return
	}
	call.summary = summary
	o.emitEv(Event{
		ID:       call.id,
		Time:     call.start,
		Category: "DISPATCH",
		Agent:    "coordinator",
		Summary:  summary,
		Level:    "info",
		Depth:    call.depth,
	})
}

func (o *observer) updateToolCallSummaryFromDelta(agent, tool, delta string) {
	key := streamArgKey(agent, tool)
	prefix := o.streamArgPrefixes[key] + delta
	if len(prefix) > 512 {
		prefix = prefix[:512]
	}
	o.streamArgPrefixes[key] = prefix

	summary := streamedToolLabel(tool, prefix)
	if summary == "" {
		return
	}
	if o.streamArgLabels[key] == summary {
		return
	}
	o.streamArgLabels[key] = summary
	o.updateToolCallSummary(agent, tool, summary)
}

func streamArgKey(agent, tool string) string {
	return agent + "\x00" + tool
}

func streamedToolLabel(tool, delta string) string {
	if tool != "save_foundation" || delta == "" {
		return ""
	}
	typ := firstJSONStringField(delta, "type")
	if typ == "" {
		return ""
	}
	return fmt.Sprintf("%s[%s]", tool, typ)
}

func firstJSONStringField(raw, field string) string {
	needle := `"` + field + `"`
	idx := strings.Index(raw, needle)
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(needle):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return ""
	}
	rest = strings.TrimLeft(rest[colon+1:], " \t\r\n")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	var value strings.Builder
	escape := false
	for i := 1; i < len(rest); i++ {
		c := rest[i]
		if escape {
			value.WriteByte(c)
			escape = false
			continue
		}
		switch c {
		case '\\':
			escape = true
		case '"':
			return value.String()
		default:
			value.WriteByte(c)
		}
	}
	return ""
}

func (o *observer) handleThinkingProgress(ev agentcore.Event) {
	agent := ev.Progress.Agent
	thinking := ev.Progress.Thinking
	if agent == "" || thinking == "" {
		return
	}

	prev := o.lastThinkingByAgent[agent]
	delta := thinking
	if strings.HasPrefix(thinking, prev) {
		delta = thinking[len(prev):]
	}
	o.lastThinkingByAgent[agent] = thinking
	if delta == "" {
		return
	}
	o.emitStreamDelta(delta, true)
}

func (o *observer) handleContextProgress(ev agentcore.Event) {
	if ev.Progress == nil || len(ev.Progress.Meta) == 0 {
		return
	}
	var payload struct {
		Tokens        int     `json:"tokens"`
		ContextWindow int     `json:"context_window"`
		Percent       float64 `json:"percent"`
		Scope         string  `json:"scope"`
		Strategy      string  `json:"strategy"`
	}
	if json.Unmarshal(ev.Progress.Meta, &payload) != nil {
		return
	}

	agent := ev.Progress.Agent
	if agent == "" {
		agent = "coordinator"
	}

	// Cập nhật ảnh chụp nhanh tác nhân (thanh bên TUI luôn hiển thị)
	o.updateAgent(agent, func(a *agentState) {
		a.context = AgentContextSnapshot{
			Tokens:        payload.Tokens,
			ContextWindow: payload.ContextWindow,
			Percent:       payload.Percent,
			Scope:         payload.Scope,
			Strategy:      payload.Strategy,
		}
	})

	level := "info"
	if payload.Percent > 85 {
		level = "warn"
	}
	summary := fmt.Sprintf("%s Bối cảnh %.0f%% (%d/%d) Chiến lược: %s", agent, payload.Percent, payload.Tokens, payload.ContextWindow, payload.Strategy)

	depth := 0
	if agent != "coordinator" {
		depth = 1
	}

	if payload.Strategy != "" {
		// Quá trình nén được kích hoạt → luồng sự kiện + nhật ký
		ctxEv := Event{Time: time.Now(), Category: "SYSTEM", Agent: agent, Summary: summary, Level: level, Depth: depth}
		o.emitEv(ctxEv)
		o.persistEvent(ctxEv)
	} else {
		// Báo cáo sử dụng thông thường → Chỉ đăng nhập
		slogLevel := slog.LevelInfo
		if level == "warn" {
			slogLevel = slog.LevelWarn
		}
		slog.Log(context.Background(), slogLevel, summary, "module", "context", "agent", agent)
	}
}

func (o *observer) emitCallFinish(call *activeCall, category, agentName string, failed bool) {
	if call == nil {
		return
	}
	level := "success"
	if failed {
		level = "error"
	}
	finishEv := Event{
		ID:         call.id,
		Time:       call.start,
		FinishedAt: time.Now(),
		Failed:     failed,
		Category:   category,
		Agent:      agentName,
		Summary:    call.summary,
		Level:      level,
		Depth:      call.depth,
		Duration:   time.Since(call.start),
	}
	o.emitEv(finishEv)
	o.persistEvent(finishEv)
}

func (o *observer) flushActiveCalls(failed bool) {
	for target, call := range o.dispatchStarts {
		o.emitCallFinish(call, "DISPATCH", target, failed)
		delete(o.dispatchStarts, target)
	}
	for agent, call := range o.toolStarts {
		o.emitCallFinish(call, "TOOL", agent, failed)
		delete(o.toolStarts, agent)
	}
	clear(o.streamExtractors)
	clear(o.streamArgPrefixes)
	clear(o.streamArgLabels)
	o.currentDispatchTarget = ""
}

func (o *observer) handleToolEnd(ev agentcore.Event) {
	agent := agentFromEvent(ev)
	// Kết thúc công cụ: chuyển trạng thái về không hoạt động, nếu không thanh bên sẽ luôn hoạt động.
	// Khi việc gửi tác nhân phụ kết thúc, trạng thái của DispatchTarget sẽ bị xóa riêng bên dưới.
	o.updateAgent(agent, func(a *agentState) {
		a.tool = ""
		a.state = "idle"
	})
	delete(o.lastThinkingByAgent, agent)

	// Nhận bản ghi DISPATCH đang diễn ra (ev.Args của handToolEnd có thể trống, lấy nó từ currentDispatchTarget)
	var dispatchCall *activeCall
	var dispatchTarget string
	if ev.Tool == "subagent" {
		dispatchTarget = o.currentDispatchTarget
		o.currentDispatchTarget = ""
		if dispatchTarget == "" {
			if sub := parseSubagentArgs(ev.Args); sub.agent != "" {
				dispatchTarget = sub.agent
			}
		}
		if dispatchTarget == "" {
			dispatchTarget = "subagent"
		}
		if call, ok := o.dispatchStarts[dispatchTarget]; ok {
			dispatchCall = call
			delete(o.dispatchStarts, dispatchTarget)
		}
		// Công văn kết thúc: đặt lại trạng thái tác nhân phụ thành không hoạt động (các đường dẫn thành công/thất bại/lỗi yêu cầu việc dọn dẹp này)
		if dispatchTarget != "subagent" {
			o.updateAgent(dispatchTarget, func(a *agentState) {
				a.state = "idle"
				a.tool = ""
			})
		}
	}

	// Xóa các bản ghi đang thực hiện khỏi công cụ trực tiếp của điều phối viên (không phải đại lý phụ) (hiếm, nhưng được đảm bảo nhất quán)
	var toolCall *activeCall
	if ev.Tool != "subagent" {
		if call, ok := o.toolStarts[agent]; ok {
			toolCall = call
			delete(o.toolStarts, agent)
		}
	}

	// Trạng thái hoàn thành cuộc gọi thống nhất (thành công/thất bại), cập nhật hàng gốc có cùng ID
	emitFinish := func(call *activeCall, category, agentName string, failed bool) {
		o.emitCallFinish(call, category, agentName, failed)
	}
	emitDispatchFinish := func(failed bool) {
		emitFinish(dispatchCall, "DISPATCH", dispatchTarget, failed)
	}
	emitToolFinish := func(failed bool) {
		emitFinish(toolCall, "TOOL", agent, failed)
	}
	// Điểm mấu chốt: Nếu tác nhân phụ kết thúc, vẫn còn các lệnh gọi TOOL chưa hoàn thành bên trong tác nhân phụ (chẳng hạn như EnsureSubagentToolStarted
	// Sự kiện đang diễn ra đã được gửi trước, nhưng sau đó việc hủy bỏ/hủy ngữ cảnh đã ngăn ProgressToolEnd đến),
	// Buộc kết thúc ở đây để ngăn dòng CÔNG CỤ "đang tiến hành" mãi mãi. Trạng thái được đồng bộ hóa với công văn.
	flushOrphanSubagentTool := func(failed bool) {
		if dispatchTarget == "" {
			return
		}
		call, ok := o.toolStarts[dispatchTarget]
		if !ok {
			return
		}
		delete(o.toolStarts, dispatchTarget)
		delete(o.streamExtractors, dispatchTarget)
		emitFinish(call, "TOOL", dispatchTarget, failed)
	}

	if ev.IsError {
		depth := 0
		if agent != "coordinator" {
			depth = 1
		}
		errText := ""
		if len(ev.Result) > 0 {
			errText = string(ev.Result)
		}
		// ctx-cancel xuất phát từ hoạt động hủy bỏ hoạt động của người dùng: việc dọn dẹp trạng thái vẫn cần được thực hiện (dòng công văn/công cụ phải trở về trạng thái đã hoàn thành),
		// Nhưng hãy bỏ qua dòng ERROR độc lập + nhật ký lỗi, nhất quán với đường dẫn EventError.
		if o.isCancellationNoise(nil, errText) {
			slog.Debug("suppressed cancel-derived tool error", "module", "agent", "tool", ev.Tool, "msg", errText)
			flushOrphanSubagentTool(true)
			emitDispatchFinish(true)
			emitToolFinish(true)
			return
		}
		summary := fmt.Sprintf("%s không thành công", ev.Tool)
		detail := summary
		kind := ""
		if errText != "" {
			kind = errorKind(nil, errText)
			detail = fmt.Sprintf("%s → %s: %s", agent, ev.Tool, errText)
			summary += ": " + truncate(errText, 120)
		}
		flushOrphanSubagentTool(true)
		emitDispatchFinish(true)
		emitToolFinish(true)
		errEv := Event{
			Time:     time.Now(),
			Category: "ERROR",
			Agent:    agent,
			Summary:  summary,
			Detail:   detail,
			Kind:     kind,
			Level:    "error",
			Depth:    depth,
		}
		o.emitEv(errEv)
		o.persistEvent(errEv)
		return
	}

	if errEv, fullErr := o.subagentResultErrorEvent(ev); errEv != nil {
		if o.isCancellationNoise(nil, fullErr) {
			slog.Debug("suppressed cancel-derived subagent error", "module", "agent", "tool", ev.Tool, "msg", fullErr)
			flushOrphanSubagentTool(true)
			emitDispatchFinish(true)
			return
		}
		if dispatchTarget != "" && dispatchTarget != "subagent" {
			errEv.Agent = dispatchTarget
		}
		flushOrphanSubagentTool(true)
		emitDispatchFinish(true)
		o.emitEv(*errEv)
		o.persistEvent(*errEv)
		return
	}

	// Đại lý con đã hoàn tất thành công → Cập nhật trạng thái hoàn thành hành vi DISPATCH ban đầu (có tiêu tốn thời gian)
	if ev.Tool == "subagent" {
		flushOrphanSubagentTool(false)
		emitDispatchFinish(false)
		return
	}

	// công cụ điều phối trực tiếp đã hoàn thành thành công
	emitToolFinish(false)
}

func (o *observer) emitStreamDelta(delta string, thinking bool) {
	if delta == "" {
		return
	}
	if thinking != o.streamThinking {
		o.emitD(utils.ThinkingSep)
		o.streamThinking = thinking
	}
	o.emitD(delta)
	o.streamHasContent = true
	o.streamLastByte = delta[len(delta)-1]
}

// Đảm bảoSubagentToolStarted Khi tool_call được nhận dạng lần đầu tiên bằng cách phát trực tuyến, hãy tiến hành đại lý
// Đăng ký cuộc gọi TOOL đang diễn ra để vòng quay của luồng sự kiện ghi đè "LLM streaming tool_call
// Tham số "khoảng thời gian này (thường chiếm 99% tổng thời gian cuộc gọi). args chưa hoàn thiện tại thời điểm này và tạm thời được đặt tên là một công cụ thuần túy
// là tóm tắt; bản tóm tắt với các tham số sẽ được hoàn thành khi ProgressToolStart thực sự xuất hiện.
func (o *observer) ensureSubagentToolStarted(agent, tool string) {
	if agent == "" || tool == "" {
		return
	}
	if _, ok := o.toolStarts[agent]; ok {
		return // Đã có một cuộc gọi đang diễn ra, bình thường
	}
	o.resetStreamArgLabel(agent, tool)
	id := nextEventID()
	o.toolStarts[agent] = &activeCall{
		id:      id,
		start:   time.Now(),
		summary: tool, // Trước tiên hãy sử dụng tên công cụ thuần túy và có thể cập nhật nó thành công cụ khi ProgressToolStart xuất hiện (Chương N)
		depth:   1,
	}
	o.emitAndLog(Event{
		ID:       id,
		Time:     time.Now(),
		Category: "TOOL",
		Agent:    agent,
		Summary:  tool,
		Level:    "info",
		Depth:    1,
	})
	o.updateAgent(agent, func(a *agentState) {
		a.state = "working"
		a.tool = tool
	})
	o.emitFallbackStreamHeader(tool)
}

func (o *observer) resetStreamArgLabel(agent, tool string) {
	key := streamArgKey(agent, tool)
	delete(o.streamArgPrefixes, key)
	delete(o.streamArgLabels, key)
}

// phátFallbackStreamHeader thêm tiêu đề dòng ✻ vào bảng điều khiển luồng cho các công cụ không được định cấu hình bằng trình trích xuất.
// Tất cả ba đường dẫn phải được gọi để đảm bảo tính nhất quán:
//  1. EnsureSubagentToolStarted - công cụ phát trực tuyến đại lý phụ (DeltaToolCall)
//  2. handToolUpdate ProgressToolStart —— công cụ không phát trực tuyến của đại lý phụ lập luận
//  3. handToolStart——công cụ riêng của điều phối viên
//
// Nếu thiếu bất kỳ một trong số chúng, công cụ tương tự sẽ là "người viết điều chỉnh bằng ✻, người điều phối điều chỉnh không có ✻" hoặc ngược lại.
func (o *observer) emitFallbackStreamHeader(tool string) {
	if _, has := toolDisplays[tool]; has {
		return // Có một trình trích xuất và tiêu đề được xuất ra bởi chính trình trích xuất.
	}
	o.streamClear()
	o.emitStreamDelta(streamHeaderFallback(tool)+"\n", false)
}

// StreamHeaderFallback tạo văn bản tiêu đề phát trực tuyến cho các công cụ mà không cần định cấu hình trình trích xuất.
// Cho phép người dùng xem "cái gì đang được gọi" ngay cả đối với các công cụ đọc nhẹ.
//
// Tiền tố "✻" là dấu hiệu "khối điều phối tác nhân" thông thường - renderStreamContent của TUI hãy xem điều này
// Tiền tố sẽ được hiển thị dọc theo đường dẫn renderAgentBlock (biểu tượng + nhãn được tô sáng + dòng phân cách),
// Nếu không, đường dẫn khối văn bản sẽ sử dụng màu mặc định của terminal và tiêu đề sẽ trông giống như văn bản thông thường và không bắt mắt.
func streamHeaderFallback(tool string) string {
	label := tool
	switch tool {
	case "ask_user":
		label = "Đặt câu hỏi cho người dùng"
	}
	return "✻ " + label
}

// StreamClear thông báo cho TUI để bắt đầu một vòng phát trực tiếp mới và đặt lại trạng thái liên quan đến việc phân tách đoạn văn.
// Về mặt logic, vòng mới là một "luồng trống", nếu không, trình trích xuất đầu tiên phát ra lần sau sẽ thêm nhầm các dòng trống ở đầu.
//
// StreamThinking cũng phải được đặt lại: detectStreamDelta theo dõi các cuộc gọi bằng cách sử dụng StreamThinking
// Đoạn trước không nói về suy nghĩ. Không có gì được đưa ra trong vòng mới. Lần sau phát ra (suy nghĩ = sai)
// ThoughtSep không nên được chèn vào nữa. Nếu không, tiêu đề dự phòng (chẳng hạn như ✻ đọc chương) sẽ được thay thế bằng \x02
// Ưu tiên, HasPrefix("✻") của renderStreamContent không khớp, toàn bộ đoạn văn sẽ đi đến đường dẫn văn bản
// Sau đó, nó được ThinkSep chia thành các phân đoạn suy nghĩ và màu tiêu đề được sơn làm màu suy nghĩ.
func (o *observer) streamClear() {
	o.emitC()
	o.streamHasContent = false
	o.streamLastByte = 0
	o.streamThinking = false
	// ProgressToolEnd đã bị xóa trước khi vòng tác nhân phụ cuối cùng kết thúc, vì vậy nó được xóa một cách phòng thủ.
	if len(o.streamExtractors) > 0 {
		o.streamExtractors = make(map[string]*agentExtractor)
	}
}

func (o *observer) subagentResultErrorEvent(ev agentcore.Event) (*Event, string) {
	if ev.Tool != "subagent" || len(ev.Result) == 0 {
		return nil, ""
	}
	sub := parseSubagentArgs(ev.Args)
	errMsg := parseSubagentResultError(ev.Result)
	if errMsg == "" {
		return nil, ""
	}

	target := "subagent"
	if sub.agent != "" {
		target = sub.agent
	}
	fullErr := fmt.Sprintf("%s không thành công: %s", target, errMsg)
	return &Event{
		Time:     time.Now(),
		Category: "ERROR",
		Agent:    "coordinator",
		Summary:  fmt.Sprintf("%s không thành công: %s", target, truncate(errMsg, 120)),
		Detail:   fullErr,
		Kind:     errorKind(nil, errMsg),
		Level:    "error",
	}, fullErr
}

func (o *observer) updateAgent(name string, fn func(*agentState)) {
	if name == "" {
		return
	}
	o.agentMu.Lock()
	defer o.agentMu.Unlock()
	a, ok := o.agents[name]
	if !ok {
		a = &agentState{name: name, state: "idle"}
		o.agents[name] = a
	}
	fn(a)
	a.updated = time.Now()
}

func (o *observer) agentSnapshots() []AgentSnapshot {
	o.agentMu.Lock()
	defer o.agentMu.Unlock()
	snaps := make([]AgentSnapshot, 0, len(o.agents))
	for _, a := range o.agents {
		snaps = append(snaps, AgentSnapshot{
			Name:      a.name,
			State:     a.state,
			Summary:   a.summary,
			Tool:      a.tool,
			Turn:      a.turn,
			Context:   a.context,
			UpdatedAt: a.updated,
		})
	}
	return snaps
}

func agentFromEvent(ev agentcore.Event) string {
	if ev.Progress != nil && ev.Progress.Agent != "" {
		return ev.Progress.Agent
	}
	return "coordinator"
}

func displayToolName(tool string, args json.RawMessage) string {
	if len(args) == 0 {
		return tool
	}
	switch tool {
	case "save_foundation":
		var p struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(args, &p) == nil && p.Type != "" {
			return fmt.Sprintf("%s[%s]", tool, p.Type)
		}
	case "commit_chapter", "plan_chapter", "draft_chapter", "check_consistency":
		var p struct {
			Chapter int `json:"chapter"`
		}
		if json.Unmarshal(args, &p) == nil && p.Chapter > 0 {
			return fmt.Sprintf("%s (Chương %d)", tool, p.Chapter)
		}
	case "save_review":
		var p struct {
			Chapter int    `json:"chapter"`
			Scope   string `json:"scope"`
			Verdict string `json:"verdict"`
		}
		if json.Unmarshal(args, &p) == nil {
			label := ""
			switch p.Scope {
			case "arc":
				label = "Vòng cung này"
			case "global":
				label = "tình hình chung"
			default:
				if p.Chapter > 0 {
					label = fmt.Sprintf("Chương %d", p.Chapter)
				}
			}
			if label == "" {
				return tool
			}
			if p.Verdict != "" {
				return fmt.Sprintf("%s(%s·%s)", tool, label, p.Verdict)
			}
			return fmt.Sprintf("%s(%s)", tool, label)
		}
	case "novel_context":
		var p struct {
			Chapter int `json:"chapter"`
		}
		if json.Unmarshal(args, &p) == nil && p.Chapter > 0 {
			return fmt.Sprintf("%s (Chương %d)", tool, p.Chapter)
		}
	case "read_chapter":
		var p struct {
			Chapter   int    `json:"chapter"`
			Source    string `json:"source"`
			Character string `json:"character"`
		}
		if json.Unmarshal(args, &p) == nil && p.Chapter > 0 {
			suffix := ""
			if p.Character != "" {
				suffix = "·" + p.Character + "đối thoại"
			} else if p.Source == "draft" {
				suffix = "·bản nháp"
			}
			return fmt.Sprintf("%s (Chương %d %s)", tool, p.Chapter, suffix)
		}
	}
	return tool
}

type subagentInvocation struct {
	agent string
	task  string
}

func parseSubagentResultError(result json.RawMessage) string {
	if len(result) == 0 {
		return ""
	}
	// Lỗi chính: đối tượng {"error": "..."} (tác nhân không xác định/mô hình không hợp lệ/thực thi tác nhân phụ không thành công)
	var obj struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(result, &obj); err == nil && obj.Error != "" {
		return obj.Error
	}
	// Trả về lỗi chuỗi trần tương thích với Agentcore SubAgentTool:
	// "Invalid parameters: ..." / "background mode requires ..." / "Too many parallel tasks ..."
	// Đây là các lỗi xác minh tham số lớp công cụ, is_error=false, nhưng nội dung là mô tả lỗi, cần được xác định là lỗi để tránh bị đánh giá sai là thành công.
	var s string
	if json.Unmarshal(result, &s) == nil && isSubagentErrorString(s) {
		return s
	}
	return ""
}

var subagentErrorPrefixes = []string{
	"Invalid parameters",
	"background mode requires",
	"Too many parallel tasks",
}

func isSubagentErrorString(s string) bool {
	for _, p := range subagentErrorPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func parseSubagentArgs(args json.RawMessage) subagentInvocation {
	if len(args) == 0 {
		return subagentInvocation{}
	}
	var p struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}
	if json.Unmarshal(args, &p) == nil && p.Agent != "" {
		return subagentInvocation{agent: p.Agent, task: p.Task}
	}
	return subagentInvocation{}
}
