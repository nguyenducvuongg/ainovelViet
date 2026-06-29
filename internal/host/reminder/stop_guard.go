package reminder

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/host/flow"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// StopGuard là tuyến phòng thủ cuối cùng dành cho các hệ thống "không thể ngăn cản về mặt vật lý".
// Khi LLM cố gắng end_turn:
//   - Tiến độ.Phase = Hoàn thành → Phát hành
//   - Nếu không, hãy đưa tin nhắn của người dùng và để đại lý tiếp tục đến lượt tiếp theo
//   - Chặn liên tục vượt quá mức tối đaSố lần liên tiếp → Nâng cao kết thúc quá trình chạy (biểu thị lời nhắc/nhắc nhở lỗi nghiêm trọng)
//
// Guard duy trì nội bộ số khối liên tiếp; nó được đặt lại về 0 khi phát hành thành công hoặc tiêm thành công.
// Điều thực sự thúc đẩy hành vi của Điều phối viên là Lời nhắc + Lời nhắc, StopGuard chỉ là sự che đậy.
const maxConsecutiveBlocks = 5

// NewStopGuard xây dựng StopGuard dành riêng cho Điều phối viên.
// onBlock là tùy chọn. Nếu khác không, nó sẽ được gọi một lần mỗi lần bị chặn. Nó được sử dụng để kiểm toán.
func NewStopGuard(st *store.Store, onBlock func(reason string, consecutive int32)) agentcore.StopGuard {
	var consecutive atomic.Int32
	var lastBlockTurn atomic.Int64 // TurnIndex của khối cuối cùng; -1 có nghĩa là nó chưa bị chặn
	lastBlockTurn.Store(-1)
	return func(_ context.Context, info agentcore.StopInfo) agentcore.StopDecision {
		progress, _ := st.Progress.Load()
		if progress != nil && progress.Phase == domain.PhaseComplete {
			consecutive.Store(0)
			lastBlockTurn.Store(-1)
			return agentcore.StopDecision{Allow: true}
		}
		// Chỉ khi "các ngã rẽ liền kề bị chặn liên tục" thì số lượng mới được cộng dồn; nếu không nó sẽ được coi là một vòng mới (LLM đã thực hiện các cuộc gọi công cụ và đạt được tiến bộ,
		// Hoặc người dùng chèn /resume khiến TurnIndex chảy ngược), đặt lại số đếm.
		last := lastBlockTurn.Load()
		if last < 0 || int64(info.TurnIndex) != last+1 {
			consecutive.Store(0)
		}
		lastBlockTurn.Store(int64(info.TurnIndex))
		n := consecutive.Add(1)
		if n > maxConsecutiveBlocks {
			slog.Error("stop_guard liên tục chặn giới hạn và nâng cấp lên mức chấm dứt.",
				"module", "host.reminder", "turn", info.TurnIndex, "consecutive", n)
			if onBlock != nil {
				onBlock("escalated", n)
			}
			return agentcore.StopDecision{Allow: false, Escalate: true}
		}
		inject := blockMessage(st, progress)
		if progress != nil && len(progress.PendingRewrites) > 0 {
			inject = fmt.Sprintf("Việc kết thúc cuộc trò chuyện bị cấm. Hàng đợi viết lại chưa được xóa: %v, hãy gọi ngay cho người viết để xử lý.", progress.PendingRewrites)
		}
		slog.Warn("stop_guard chặn end_turn",
			"module", "host.reminder", "turn", info.TurnIndex, "consecutive", n)
		if onBlock != nil {
			onBlock("blocked", n)
		}
		return agentcore.StopDecision{Allow: false, InjectMessage: inject}
	}
}

func blockMessage(st *store.Store, progress *domain.Progress) string {
	if progress != nil && flow.Route(flow.LoadState(st)) != nil {
		return "Việc kết thúc cuộc trò chuyện bị cấm. Giai đoạn chưa hoàn thành; vui lòng đợi và thực thi [Lệnh do máy chủ ban hành] do Máy chủ đưa ra và không tự gọi tiểu thuyết_context hoặc tác nhân phụ."
	}
	return "Việc kết thúc cuộc trò chuyện bị cấm. Giai đoạn này vẫn chưa được hoàn thành và hiện chưa có hướng dẫn định tuyến Máy chủ; đây là kịch bản điều phối viên, vui lòng tiếp tục xử lý theo các quy tắc điều phối của điều phối viên.md và không chờ hướng dẫn của Máy chủ."
}
