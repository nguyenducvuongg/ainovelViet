package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	storepkg "github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// Người trợ giúp: Xây dựng Tiến trình trong giai đoạn Viết và chế độ phân cấp.
func writingProgress(completed []int, flow domain.FlowState) *domain.Progress {
	return &domain.Progress{
		Phase:             domain.PhaseWriting,
		Flow:              flow,
		Layered:           true,
		CompletedChapters: completed,
	}
}

func TestRoute_NilProgress(t *testing.T) {
	if got := Route(State{Progress: nil}); got != nil {
		t.Fatalf("expected nil for nil progress, got %+v", got)
	}
}

func TestRoute_PhaseComplete(t *testing.T) {
	s := State{Progress: &domain.Progress{Phase: domain.PhaseComplete}}
	if got := Route(s); got != nil {
		t.Fatalf("expected nil at PhaseComplete, got %+v", got)
	}
}

func TestRoute_NonWritingPhasesDelegateToLLM(t *testing.T) {
	for _, phase := range []domain.Phase{domain.PhaseInit, domain.PhasePremise, domain.PhaseOutline} {
		s := State{Progress: &domain.Progress{Phase: phase}, FoundationMissing: []string{"premise"}}
		if got := Route(s); got != nil {
			t.Fatalf("phase %s should return nil, got %+v", phase, got)
		}
	}
}

func TestRoute_PendingRewritesFirst(t *testing.T) {
	p := writingProgress([]int{1, 2}, domain.FlowRewriting)
	p.PendingRewrites = []int{3, 5}
	got := Route(State{Progress: p})
	if got == nil || got.Agent != "writer" {
		t.Fatalf("expected writer for rewrites, got %+v", got)
	}
	if got.Task != "viết lại Chương 3" {
		t.Errorf("mong đợi 'viết lại Chương 3', nhận %q", got.Task)
	}
	if got.Chapter != 3 {
		t.Errorf("expected Chapter=3, got %d", got.Chapter)
	}
}

func TestRoute_PendingPolishingVerb(t *testing.T) {
	p := writingProgress([]int{1}, domain.FlowPolishing)
	p.PendingRewrites = []int{2}
	got := Route(State{Progress: p})
	if got == nil || got.Task != "đánh bóng Chương 2" {
		t.Fatalf("expected polish verb, got %+v", got)
	}
}

func TestRoute_ReviewingDelegatesToLLM(t *testing.T) {
	p := writingProgress([]int{1, 2}, domain.FlowReviewing)
	if got := Route(State{Progress: p}); got != nil {
		t.Fatalf("expected nil during reviewing, got %+v", got)
	}
}

func TestRoute_SteeringDelegatesToLLM(t *testing.T) {
	p := writingProgress([]int{1}, domain.FlowSteering)
	if got := Route(State{Progress: p}); got != nil {
		t.Fatalf("expected nil during steering, got %+v", got)
	}
}

func TestRoute_ArcEndNeedsReview(t *testing.T) {
	p := writingProgress([]int{10}, domain.FlowWriting)
	s := State{
		Progress:      p,
		LastCompleted: 10,
		ArcBoundary: &storepkg.ArcBoundary{
			IsArcEnd: true,
			Volume:   1,
			Arc:      2,
		},
	}
	got := Route(s)
	if got == nil || got.Agent != "editor" {
		t.Fatalf("expected editor for arc review, got %+v", got)
	}
	if got.Reason != "Đánh giá cuối phần chưa hoàn thành" {
		t.Errorf("reason mismatch: %q", got.Reason)
	}
}

func TestRoute_ArcEndHasReviewNeedsSummary(t *testing.T) {
	p := writingProgress([]int{10}, domain.FlowWriting)
	s := State{
		Progress:      p,
		LastCompleted: 10,
		ArcBoundary: &storepkg.ArcBoundary{
			IsArcEnd: true,
			Volume:   1,
			Arc:      2,
		},
		HasArcReview: true,
	}
	got := Route(s)
	if got == nil || got.Agent != "editor" || got.Reason != "Tóm tắt Arc chưa hoàn thành" {
		t.Fatalf("expected arc summary editor call, got %+v", got)
	}
}

func TestRoute_VolumeEndNeedsVolumeSummary(t *testing.T) {
	p := writingProgress([]int{20}, domain.FlowWriting)
	s := State{
		Progress:      p,
		LastCompleted: 20,
		ArcBoundary: &storepkg.ArcBoundary{
			IsArcEnd:    true,
			IsVolumeEnd: true,
			Volume:      1,
			Arc:         3,
		},
		HasArcReview:  true,
		HasArcSummary: true,
	}
	got := Route(s)
	if got == nil || got.Reason != "Tóm tắt tập chưa hoàn thành" {
		t.Fatalf("expected volume summary request, got %+v", got)
	}
}

func TestRoute_NeedsArcExpansion(t *testing.T) {
	p := writingProgress([]int{10}, domain.FlowWriting)
	s := State{
		Progress:      p,
		LastCompleted: 10,
		ArcBoundary: &storepkg.ArcBoundary{
			IsArcEnd:       true,
			Volume:         1,
			Arc:            2,
			NextVolume:     1,
			NextArc:        3,
			NeedsExpansion: true,
		},
		HasArcReview:  true,
		HasArcSummary: true,
	}
	got := Route(s)
	if got == nil || got.Agent != "architect_long" {
		t.Fatalf("expected architect_long for expansion, got %+v", got)
	}
	if got.Reason != "Bộ xương vòng cung tiếp theo sẽ được mở rộng" {
		t.Errorf("reason mismatch: %q", got.Reason)
	}
}

func TestRoute_NeedsNewVolume(t *testing.T) {
	p := writingProgress([]int{30}, domain.FlowWriting)
	s := State{
		Progress:      p,
		LastCompleted: 30,
		ArcBoundary: &storepkg.ArcBoundary{
			IsArcEnd:       true,
			IsVolumeEnd:    true,
			Volume:         2,
			Arc:            4,
			NeedsNewVolume: true,
		},
		HasArcReview:     true,
		HasArcSummary:    true,
		HasVolumeSummary: true,
	}
	got := Route(s)
	if got == nil || got.Agent != "architect_long" || got.Reason != "Khi kết thúc tập, bạn phải quyết định nên thêm tập mới hay kết thúc cuốn sách." {
		t.Fatalf("expected append_volume/complete_book dispatch, got %+v", got)
	}
}

func TestRoute_NormalContinue(t *testing.T) {
	p := writingProgress([]int{1, 2, 3}, domain.FlowWriting)
	p.TotalChapters = 20
	got := Route(State{Progress: p, LastCompleted: 3})
	if got == nil || got.Agent != "writer" {
		t.Fatalf("expected writer for next chapter, got %+v", got)
	}
	if got.Task != "Viết chương 4" {
		t.Errorf("dự kiến ​​'Viết chương 4', nhận %q", got.Task)
	}
	if got.Chapter != 4 {
		t.Errorf("expected Chapter=4, got %d", got.Chapter)
	}
}

func TestRoute_ArcEndNonLayeredSkipsBoundary(t *testing.T) {
	// Chế độ không phân lớp không lấy nhánh cuối cung ngay cả khi ArcBoundary không phải là số 0
	p := &domain.Progress{
		Phase:             domain.PhaseWriting,
		Flow:              domain.FlowWriting,
		Layered:           false,
		CompletedChapters: []int{10},
		TotalChapters:     20,
	}
	s := State{
		Progress:      p,
		LastCompleted: 10,
		ArcBoundary:   &storepkg.ArcBoundary{IsArcEnd: true, Volume: 1, Arc: 2},
	}
	got := Route(s)
	if got == nil || got.Agent != "writer" {
		t.Fatalf("non-layered should fall through to writer, got %+v", got)
	}
}

func TestFormatMessage(t *testing.T) {
	msg := FormatMessage(&Instruction{Agent: "writer", Task: "Viết chương 5", Reason: "Tiếp tục viết"})
	for _, want := range []string{
		"[Hướng dẫn vấn đề máy chủ]",
		"tác nhân phụ (writer, \"Viết chương 5\")",
		"agent: writer",
		"task: \"Viết chương 5\"",
		"Tiếp tục viết",
		"phải sử dụng tác nhân/tác vụ trên như hiện tại",
		"không viết lại tác vụ",
		"không điều chỉnh tiểu thuyết_context trước",
	} {
		if !contains(msg, want) {
			t.Errorf("message missing %q: %s", want, msg)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDispatcher_TrackRepeat(t *testing.T) {
	// Không cần điều phối viên/cửa hàng thực sự; trackRepeat chỉ đọc bộ đệm của chính nó.
	d := &Dispatcher{}
	inst := &Instruction{Agent: "writer", Task: "Viết chương 5", Reason: "Tiếp tục viết"}
	if got := d.trackRepeat(inst); got != 1 {
		t.Fatalf("Lần phát hành đầu tiên tích lũy 1, nhận được %d", got)
	}
	if got := d.trackRepeat(inst); got != 2 {
		t.Fatalf("Lặp lại với Tác nhân+Nhiệm vụ để giải phóng tích lũy 2, nhận %d", got)
	}
	// Khi Lý do khác và Tác nhân+Nhiệm vụ giống nhau thì được coi là cùng một lệnh và tiếp tục tích lũy.
	sameTaskDiffReason := &Instruction{Agent: "writer", Task: "Viết chương 5", Reason: "Tiếp tục sau khi kết thúc arc"}
	if got := d.trackRepeat(sameTaskDiffReason); got != 3 {
		t.Fatalf("Chỉ có sự khác biệt về lý do nên được coi là trùng lặp được tích lũy thành 3, có %d", got)
	}
	other := &Instruction{Agent: "writer", Task: "Viết chương 6", Reason: "Tiếp tục viết"}
	if got := d.trackRepeat(other); got != 1 {
		t.Fatalf("Tác vụ phải được đặt lại về 1 sau khi thay đổi, nhận được %d", got)
	}
	d.ResetRepeat()
	if got := d.trackRepeat(other); got != 1 {
		t.Fatalf("Tích lũy đầu tiên sau ResetRepeat 1, nhận được %d", got)
	}
}

func TestFormatDispatchMessage_RepeatNotice(t *testing.T) {
	inst := &Instruction{Agent: "writer", Task: "Viết chương 5", Reason: "Tiếp tục viết"}
	first := formatDispatchMessage(inst, 1)
	if first != FormatMessage(inst) {
		t.Fatalf("Bản phát hành đầu tiên không được có ghi chú trùng lặp: %s", first)
	}
	third := formatDispatchMessage(inst, 3)
	for _, want := range []string{"lần 3", "các dữ kiện định tuyến không thay đổi", "tiểu thuyết_context", "tác nhân phụ khác"} {
		if !contains(third, want) {
			t.Errorf("Chú thích trùng lặp bị thiếu %q: %s", want, third)
		}
	}
}

func TestDispatcher_OnRepeatFiresOnceAtThreshold(t *testing.T) {
	d := &Dispatcher{}
	var fired []string
	d.SetOnRepeat(func(agent, task string, n int) {
		fired = append(fired, fmt.Sprintf("%s|%s|%d", agent, task, n))
	})

	inst := &Instruction{Agent: "writer", Task: "Viết chương 5"}
	for range 6 {
		d.trackRepeat(inst) // n=1..6: Chỉ gọi lại một lần khi n==3
	}
	if len(fired) != 1 || fired[0] != fmt.Sprintf("writer|Viết chương 5|%d", repeatNotifyAt) {
		t.Fatalf("Nên kích hoạt đúng một lần tại %d, nhận được %v", repeatNotifyAt, fired)
	}

	// Sắp xếp lại sau khi thay đổi phím: thay đổi nhiệm vụ 3 lần liên tiếp → kích hoạt lại
	other := &Instruction{Agent: "writer", Task: "Viết chương 6"}
	for range 3 {
		d.trackRepeat(other)
	}
	if len(fired) != 2 {
		t.Fatalf("Nên trang bị lại sau khi đổi chìa khóa, nhận được %v", fired)
	}
}

func TestDispatcher_SteersAfterSuccessfulBoundaryToolBeforeNextModelCall(t *testing.T) {
	st := storepkg.NewStore(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := st.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	var secondReq *agentcore.LLMRequest
	var dispatcher *Dispatcher
	coordinator := agentcore.NewAgent(
		agentcore.WithModel(sequentialFlowTestModel(func(i int, req *agentcore.LLMRequest) (*agentcore.LLMResponse, error) {
			if i == 0 {
				return &agentcore.LLMResponse{Message: flowTestToolCallMsg(agentcore.ToolCall{
					ID:   "tc-subagent",
					Name: "subagent",
					Args: json.RawMessage(`{"agent":"architect_long","task":"plan"}`),
				})}, nil
			}
			secondReq = req
			return &agentcore.LLMResponse{Message: flowTestAssistantMsg("done", agentcore.StopReasonStop)}, nil
		})),
		agentcore.WithTools(agentcore.NewFuncTool("subagent", "fake subagent", map[string]any{
			"type": "object",
		}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
			if err := st.Progress.UpdatePhase(domain.PhaseWriting); err != nil {
				return nil, err
			}
			return json.RawMessage(`"foundation_ready=true"`), nil
		})),
		agentcore.WithMiddlewares(func(ctx context.Context, call agentcore.ToolCall, next agentcore.ToolExecuteFunc) (json.RawMessage, error) {
			out, err := next(ctx, call.Args)
			if err == nil && call.Name == "subagent" {
				dispatcher.Dispatch()
			}
			return out, err
		}),
	)

	dispatcher = NewDispatcher(coordinator, st)
	dispatcher.Enable()

	if err := coordinator.Prompt(context.Background(), "start"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	coordinator.WaitForIdle()

	if secondReq == nil {
		t.Fatal("expected second model request")
	}
	if len(secondReq.Messages) < 4 {
		t.Fatalf("expected tool result and Host instruction in second request, got %d messages", len(secondReq.Messages))
	}
	if result := secondReq.Messages[len(secondReq.Messages)-2]; result.Role != agentcore.RoleTool {
		t.Fatalf("expected tool result immediately before Host instruction, got %q", result.Role)
	}
	got := secondReq.Messages[len(secondReq.Messages)-1].TextContent()
	for _, want := range []string{"[Hướng dẫn vấn đề máy chủ]", "tác nhân phụ (writer", "Viết chương 1"} {
		if !contains(got, want) {
			t.Fatalf("Host instruction missing %q: %s", want, got)
		}
	}
}

type flowTestSequentialModel struct {
	fn  func(i int, req *agentcore.LLMRequest) (*agentcore.LLMResponse, error)
	idx int64
}

func sequentialFlowTestModel(fn func(i int, req *agentcore.LLMRequest) (*agentcore.LLMResponse, error)) *flowTestSequentialModel {
	return &flowTestSequentialModel{fn: fn}
}

func (m *flowTestSequentialModel) take(msgs []agentcore.Message, tools []agentcore.ToolSpec) (*agentcore.LLMResponse, error) {
	i := int(atomic.AddInt64(&m.idx, 1) - 1)
	return m.fn(i, &agentcore.LLMRequest{Messages: msgs, Tools: tools})
}

func (m *flowTestSequentialModel) Generate(_ context.Context, msgs []agentcore.Message, tools []agentcore.ToolSpec, _ ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	return m.take(msgs, tools)
}

func (m *flowTestSequentialModel) GenerateStream(_ context.Context, msgs []agentcore.Message, tools []agentcore.ToolSpec, _ ...agentcore.CallOption) (<-chan agentcore.StreamEvent, error) {
	resp, err := m.take(msgs, tools)
	if err != nil {
		return nil, err
	}
	ch := make(chan agentcore.StreamEvent, 1)
	ch <- agentcore.StreamEvent{Type: agentcore.StreamEventDone, Message: resp.Message, StopReason: resp.Message.StopReason}
	close(ch)
	return ch, nil
}

func (m *flowTestSequentialModel) SupportsTools() bool { return true }

func flowTestAssistantMsg(text string, stop agentcore.StopReason) agentcore.Message {
	return agentcore.Message{
		Role:       agentcore.RoleAssistant,
		Content:    []agentcore.ContentBlock{agentcore.TextBlock(text)},
		StopReason: stop,
	}
}

func flowTestToolCallMsg(calls ...agentcore.ToolCall) agentcore.Message {
	blocks := make([]agentcore.ContentBlock, len(calls))
	for i, call := range calls {
		blocks[i] = agentcore.ToolCallBlock(call)
	}
	return agentcore.Message{
		Role:       agentcore.RoleAssistant,
		Content:    blocks,
		StopReason: agentcore.StopReasonToolUse,
	}
}
