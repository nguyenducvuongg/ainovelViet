package reminder

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// subagentMaxConsecutiveBlocks nâng cấp để chấm dứt sau khi chặn N lần liên tục để tránh vòng lặp vô hạn của các mô hình yếu.
const subagentMaxConsecutiveBlocks = 3

// hardStopReason là các lý do từ chối phía nhà cung cấp không thể khôi phục bằng thông báo nhắc nhở. tiêm
// "Phải cam kết" không có tác dụng với họ. Thay vào đó, nó tạo ra mức tiêu thụ mã thông báo của một cuộc gọi LLM hoàn chỉnh mỗi lần.
// Và cuối cùng nâng cấp lên cấp cao hơn và cho phép điều phối viên triển khai lại toàn bộ SubAgent, chồng chất nhiều lần lãng phí.
// (Trong phép đo thực tế, khi ch02 đạt mức an toàn, việc viết một chương một lần sẽ tạo ra 3 lần lên lịch lại, 17 cuộc gọi LLM và tỷ lệ trúng.
// giảm từ 50% xuống 2,8%).
//
// Lưu ý rằng StopReasonError / StopReasonAborted không cần đưa vào: Agentcore
// Khi loop.go nhận được hai lý do dừng này, nó sẽ trực tiếp chấm dứt quá trình chạy và không gọi StopGuard chút nào.
// Chỉ những ngữ nghĩa từ chối của nhà cung cấp thực sự đạt đến StopGuard mới được liệt kê ở đây.
var hardStopReasons = map[agentcore.StopReason]struct{}{
	"safety":         {},
	"content_filter": {},
}

// newCheckpointDeltaGuard xây dựng StopGuard:
// Nếu điểm kiểm tra của bước được chỉ định không xuất hiện sau đường cơ sở, end_turn sẽ bị từ chối.
// Đường cơ sở được người gọi ghi lại tại thời điểm xuất xưởng để đảm bảo ngữ nghĩa chính xác cho mỗi lần chạy.
func newCheckpointDeltaGuard(st *store.Store, agentName string, requiredSteps []string, blockMsg string) agentcore.StopGuard {
	var baseline int64
	if cp := st.Checkpoints.LatestGlobal(); cp != nil {
		baseline = cp.Seq
	}
	need := make(map[string]struct{}, len(requiredSteps))
	for _, s := range requiredSteps {
		need[s] = struct{}{}
	}
	var consecutive atomic.Int32
	return func(_ context.Context, info agentcore.StopInfo) agentcore.StopDecision {
		// Lỗi không thể phục hồi: nâng cấp trực tiếp mà không lãng phí lời nhắc.
		if _, hard := hardStopReasons[info.Message.StopReason]; hard {
			slog.Error("tác nhân phụ stop_guard phát hiện việc tắt máy không thể phục hồi và nâng cấp ngay lập tức",
				"module", "host.reminder", "agent", agentName,
				"turn", info.TurnIndex, "stop_reason", info.Message.StopReason)
			return agentcore.StopDecision{Allow: false, Escalate: true}
		}
		// Quét theo thứ tự ngược lại: điểm kiểm tra mới ở cuối và bạn có thể ngắt khi gặp <= đường cơ sở.
		all := st.Checkpoints.All()
		for i := len(all) - 1; i >= 0; i-- {
			cp := all[i]
			if cp.Seq <= baseline {
				break
			}
			if _, ok := need[cp.Step]; ok {
				consecutive.Store(0)
				return agentcore.StopDecision{Allow: true}
			}
		}
		n := consecutive.Add(1)
		if n > subagentMaxConsecutiveBlocks {
			slog.Error("subagent stop_guard Việc chặn liên tục vượt quá giới hạn và được nâng cấp lên mức chấm dứt",
				"module", "host.reminder", "agent", agentName, "turn", info.TurnIndex, "consecutive", n)
			return agentcore.StopDecision{Allow: false, Escalate: true}
		}
		slog.Warn("tác nhân phụ stop_guard chặn end_turn",
			"module", "host.reminder", "agent", agentName, "turn", info.TurnIndex, "consecutive", n)
		return agentcore.StopDecision{Allow: false, InjectMessage: blockMsg}
	}
}

// NewWriterStopGuard yêu cầu người viết tạo ít nhất một commit_chapter thành công trong chu kỳ này.
func NewWriterStopGuard(st *store.Store) agentcore.StopGuard {
	return newCheckpointDeltaGuard(st, "writer",
		[]string{"commit"},
		"Bạn phải gọi commit_chapter để gửi chương này trước khi nó có thể kết thúc. Draft_chapter chỉ lưu bản nháp, chưa hoàn thành.",
	)
}

// NewArchitectStopGuard yêu cầu kiến ​​trúc sư đặt save_foundation ít nhất một lần trong vòng này.
func NewArchitectStopGuard(st *store.Store) agentcore.StopGuard {
	return newCheckpointDeltaGuard(st, "architect",
		[]string{
			"premise", "outline", "layered_outline", "characters", "world_rules",
			"expand_arc", "append_volume", "update_compass", "complete_book",
		},
		"Bạn phải gọi save_foundation để đặt đầu ra trước khi nó có thể kết thúc. Chỉ xuất ra văn bản Markdown/JSON bằng với mất mát.",
	)
}

// NewEditorStopGuard yêu cầu người chỉnh sửa kết thúc vòng này sau khi đặt sản phẩm phù hợp với "nhiệm vụ".
//
// Nhận thức về nhiệm vụ: Khi được gửi để tạo bản tóm tắt, chỉ save_review (đánh giá) là chưa hoàn thành - bản tóm tắt tương ứng phải được tạo.
// Nếu không, trình soạn thảo "được tạo thành bản tóm tắt vòng cung nhưng được xem xét trước" sẽ đáp ứng các tiêu chí thoải mái cũ và kết thúc sớm, và bản tóm tắt vòng cung sẽ không bao giờ được đưa vào đĩa.
// (Hợp tác với người điều phối để loại bỏ trùng lặp và bắn nhầm đã từng khiến cung cốt trong tập lặp lại không ngừng. Để biết chi tiết, hãy xem phác thảo-kiệt sức-livelock).
// Lối thoát StopAfterTool sẽ bỏ qua StopGuard (loop.go), do đó build.go sẽ di chuyển save_review ra khỏi điểm dừng cứng một cách đồng bộ.
// Sau khi xem xét, bạn có thể tiếp tục đến công cụ tóm tắt và sau đó người bảo vệ sẽ hoàn thành nó.
func NewEditorStopGuard(st *store.Store, task string) agentcore.StopGuard {
	switch {
	case strings.Contains(task, "save_volume_summary") || strings.Contains(task, "Tóm tắt tập"):
		return newCheckpointDeltaGuard(st, "editor", []string{"volume_summary"},
			"Nhiệm vụ này là tạo bản tóm tắt tập: bạn phải gọi save_volume_summary trước khi có thể hoàn thành. Quá trình xem xét save_review chưa hoàn tất.")
	case strings.Contains(task, "save_arc_summary") || strings.Contains(task, "tóm tắt vòng cung"):
		return newCheckpointDeltaGuard(st, "editor", []string{"arc_summary"},
			"Nhiệm vụ này là tạo một bản tóm tắt vòng cung: bạn phải gọi save_arc_summary trước khi đặt hàng và việc xem xét save_review chưa hoàn thành.")
	default:
		// Đánh giá hoặc bài tập đặc biệt: Có thể gửi bất kỳ đánh giá/tóm tắt nào (duy trì hành vi lỏng lẻo hiện có).
		return newCheckpointDeltaGuard(st, "editor",
			[]string{"review", "arc_summary", "volume_summary"},
			"Bạn phải gọi một trong các save_review / save_arc_summary / save_volume_summary để tải xuống kết quả trước khi nó có thể kết thúc.")
	}
}
