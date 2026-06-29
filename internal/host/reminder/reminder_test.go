package reminder

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	return s
}

func TestStopGuard_AllowsStopOnlyWhenComplete(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	guard := NewStopGuard(s, nil)

	// Chưa hoàn thành: phải chặn + tiêm
	decision := guard(context.Background(), agentcore.StopInfo{TurnIndex: 1})
	if decision.Allow {
		t.Fatal("stop must be blocked before Phase=Complete")
	}
	if decision.InjectMessage == "" {
		t.Fatal("inject message required when blocking")
	}

	// Chuyển sang hoàn thành: Phát hành
	if err := s.Progress.UpdatePhase(domain.PhaseComplete); err != nil {
		t.Fatalf("update phase: %v", err)
	}
	decision = guard(context.Background(), agentcore.StopInfo{TurnIndex: 2})
	if !decision.Allow {
		t.Fatal("stop must be allowed when Phase=Complete")
	}
}

func TestStopGuard_EscalatesAfterTooManyConsecutiveBlocks(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	var blocks []string
	guard := NewStopGuard(s, func(reason string, _ int32) {
		blocks = append(blocks, reason)
	})

	for i := 0; i < maxConsecutiveBlocks; i++ {
		decision := guard(context.Background(), agentcore.StopInfo{TurnIndex: i})
		if decision.Escalate {
			t.Fatalf("escalated too early at iteration %d", i)
		}
	}
	decision := guard(context.Background(), agentcore.StopInfo{TurnIndex: maxConsecutiveBlocks})
	if !decision.Escalate {
		t.Fatalf("expected escalate after %d consecutive blocks", maxConsecutiveBlocks+1)
	}
	if len(blocks) != maxConsecutiveBlocks+1 {
		t.Fatalf("audit callback called %d times, want %d", len(blocks), maxConsecutiveBlocks+1)
	}
	if blocks[len(blocks)-1] != "escalated" {
		t.Fatalf("last audit reason should be 'escalated', got %q", blocks[len(blocks)-1])
	}
}

func TestStopGuard_DefaultBlockMessageWaitsForHost(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	if err := s.Progress.UpdatePhase(domain.PhaseWriting); err != nil {
		t.Fatalf("update phase: %v", err)
	}

	decision := NewStopGuard(s, nil)(context.Background(), agentcore.StopInfo{TurnIndex: 1})
	if !strings.Contains(decision.InjectMessage, "[Lệnh do máy chủ ban hành]") {
		t.Fatalf("inject message should point to Host instruction, got %q", decision.InjectMessage)
	}
	for _, forbidden := range []string{"Kiểm tra tiểu thuyết_context", "Chất giai điệu"} {
		if strings.Contains(decision.InjectMessage, forbidden) {
			t.Fatalf("inject message should not suggest freelance action %q: %q", forbidden, decision.InjectMessage)
		}
	}
}

func TestStopGuard_DefaultBlockMessageAllowsCoordinatorJudgmentWhenNoRoute(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	decision := NewStopGuard(s, nil)(context.Background(), agentcore.StopInfo{TurnIndex: 1})
	if strings.Contains(decision.InjectMessage, "[Lệnh do máy chủ ban hành]") {
		t.Fatalf("no-route inject should not tell coordinator to wait for Host, got %q", decision.InjectMessage)
	}
	if !strings.Contains(decision.InjectMessage, "kịch bản điều phối viên") {
		t.Fatalf("no-route inject should mention coordinator judgment, got %q", decision.InjectMessage)
	}
}

// TestSubAgentGuard_HardStopReasonEscatesXác thực ngay lập tức: Mô hình được trả về
// Khi nhà cung cấp không thể khôi phục như safety/content_filter từ chối trả lời, tác nhân phụ StopGuard
// Phải báo cáo ngay lập tức thay vì đưa ra một thông báo nhắc nhở.
//
// Bối cảnh lịch sử: test thực tế hy3-preview:freestop_reason='safety' 8 lần liên tiếp khi viết Chương 2
// Từ chối trả lời; logic cũ liên tục đưa ra "phải cam kết", mô hình tiếp tục an toàn và các khối được lưu 3 lần trước khi tăng cấp.
// Sau đó, người điều phối đã phân công lại người viết tổng cộng 3 lần. Mỗi lần gửi lại là một Tác nhân phụ mới → bộ đệm
// Tiền tố tất cả khởi động nguội. Mức độ an toàn đầu tiên ngay lập tức tăng cao sau khi khắc phục và điều phối viên đã xóa LLM khỏi
// Thông báo lỗi cho thấy nó không thể phục hồi được và có xu hướng thay đổi đường dẫn thay vì phân phối lại.
//
// Lưu ý rằng chỉ an toàn/content_filter: StopReasonError/StopReasonAborted mới được kiểm tra
// Agentcore loop.go trực tiếp chấm dứt nhánh đang chạy và hoàn toàn không gọi StopGuard. Liệt kê nó thay thế
// Giới thiệu mã chết.
func TestSubAgentGuard_HardStopReasonEscalatesImmediately(t *testing.T) {
	cases := []agentcore.StopReason{
		agentcore.StopReason("safety"),
		agentcore.StopReason("content_filter"),
	}
	for _, sr := range cases {
		t.Run(string(sr), func(t *testing.T) {
			s := newTestStore(t)
			guard := NewWriterStopGuard(s)
			info := agentcore.StopInfo{
				TurnIndex: 1,
				Message:   agentcore.Message{StopReason: sr},
			}
			d := guard(context.Background(), info)
			if !d.Escalate {
				t.Fatalf("stop_reason=%q must escalate immediately, got %#v", sr, d)
			}
			if d.InjectMessage != "" {
				t.Fatalf("stop_reason=%q must not inject any message, got %q", sr, d.InjectMessage)
			}
		})
	}
}

// TestSubAgentGuard_NormalStopStillBlocks đảm bảo hành vi chặn đối với stop_reason thông thường
// Không bị ảnh hưởng bởi việc bỏ qua lỗi cứng - LLM vẫn cần được đẩy khi nó tự động dừng và không cam kết.
func TestSubAgentGuard_NormalStopStillBlocks(t *testing.T) {
	s := newTestStore(t)
	guard := NewWriterStopGuard(s)
	info := agentcore.StopInfo{
		TurnIndex: 1,
		Message:   agentcore.Message{StopReason: agentcore.StopReasonStop},
	}
	d := guard(context.Background(), info)
	if d.Escalate {
		t.Fatal("normal stop must not escalate on first block")
	}
	if d.Allow {
		t.Fatal("normal stop must be blocked when no commit checkpoint exists")
	}
	if d.InjectMessage == "" {
		t.Fatal("normal stop must inject a follow-up message")
	}
}

// TestStopGuard_NonConsecutiveTurnResetsCounter xác minh: TurnIndex giữa hai khối
// Khi chúng không liền kề (LLM ở giữa thực hiện lệnh gọi công cụ hoặc người dùng tiếp tục), số đếm nhất quán sẽ được đặt lại.
func TestStopGuard_NonConsecutiveTurnResetsCounter(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	guard := NewStopGuard(s, nil)

	for i := 0; i < maxConsecutiveBlocks; i++ {
		if d := guard(context.Background(), agentcore.StopInfo{TurnIndex: i}); d.Escalate {
			t.Fatalf("escalated too early at iteration %d", i)
		}
	}

	d := guard(context.Background(), agentcore.StopInfo{TurnIndex: maxConsecutiveBlocks + 10})
	if d.Escalate {
		t.Fatal("non-consecutive block must NOT escalate; counter should have been reset")
	}
	if d.Allow {
		t.Fatal("stop must still be blocked when Phase != Complete")
	}

	d = guard(context.Background(), agentcore.StopInfo{TurnIndex: 1})
	if d.Escalate {
		t.Fatal("resume (TurnIndex backflow) must NOT escalate")
	}
}

// TestEditorStopGuard_TaskAware Xác thực nhận thức về nhiệm vụ: save_review chỉ khi được lấy dưới dạng tóm tắt vòng cung
// Nó không được coi là hoàn thành và arc_summary phải được tạo trước khi phát hành - Khiếm khuyết C, điểm bắt đầu của vòng lặp vô hạn cung xương trong khối chặn.
func TestEditorStopGuard_TaskAware(t *testing.T) {
	normalStop := agentcore.StopInfo{TurnIndex: 1, Message: agentcore.Message{StopReason: agentcore.StopReasonStop}}

	// Nhiệm vụ tóm tắt + chỉ đánh giá được lưu → phải bị chặn (đánh giá không đáp ứng yêu cầu arc_summary).
	t.Run("summary task blocks on review only", func(t *testing.T) {
		s := newTestStore(t)
		guard := NewEditorStopGuard(s, "Tạo bản tóm tắt tập 5 arc 1 (save_arc_summary)")
		if _, err := s.Checkpoints.Append(domain.ArcScope(5, 1), "review", "reviews/v05a01.json", "d1"); err != nil {
			t.Fatalf("append review: %v", err)
		}
		if d := guard(context.Background(), normalStop); d.Allow {
			t.Fatal("summary task must NOT be satisfied by a review checkpoint")
		}
	})

	// Nhiệm vụ tóm tắt + arc_summary đã lưu → Phát hành.
	t.Run("summary task allows on arc_summary", func(t *testing.T) {
		s := newTestStore(t)
		guard := NewEditorStopGuard(s, "Tạo bản tóm tắt tập 5 arc 1 (save_arc_summary)")
		if _, err := s.Checkpoints.Append(domain.ArcScope(5, 1), "arc_summary", "summaries/arc-v05a01.json", "d1"); err != nil {
			t.Fatalf("append arc_summary: %v", err)
		}
		if d := guard(context.Background(), normalStop); !d.Allow {
			t.Fatal("summary task must be satisfied by an arc_summary checkpoint")
		}
	})

	// Xem lại nhiệm vụ + lưu đánh giá → phát hành (hành vi khoan dung mặc định không thay đổi).
	t.Run("review task allows on review", func(t *testing.T) {
		s := newTestStore(t)
		guard := NewEditorStopGuard(s, "Thực hiện đánh giá phần cho Tập 5, Phần 1 (scope=arc)")
		if _, err := s.Checkpoints.Append(domain.ArcScope(5, 1), "review", "reviews/v05a01.json", "d1"); err != nil {
			t.Fatalf("append review: %v", err)
		}
		if d := guard(context.Background(), normalStop); !d.Allow {
			t.Fatal("review task must be satisfied by a review checkpoint")
		}
	})
}
