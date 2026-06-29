package domain

import (
	"fmt"

	"github.com/nguyenducvuongg/ainovelViet/internal/errs"
)

// Quy tắc di chuyển tiểu bang (phiên bản tối thiểu)
//
// Giai đoạn đại diện cho một giai đoạn lớn và chấp nhận ràng buộc "chỉ tiến và không lùi":
//
//	init -> premise -> outline -> writing -> complete
//	  \---------> outline ------^
//	  \-----------------> writing
//
// Luồng đại diện cho quy trình hiện đang hoạt động, cho phép chuyển đổi trong thời gian viết, nhưng không cho phép các bước nhảy bất thường rõ ràng:
//
//	writing   -> reviewing / rewriting / polishing / steering / writing
//	reviewing -> writing / rewriting / polishing / steering / reviewing
//	rewriting -> writing / steering / rewriting
//	polishing -> writing / steering / polishing
//	steering  -> writing / reviewing / rewriting / polishing / steering
//
// Trạng thái trống (giá trị bằng 0) được coi là "chưa được khởi tạo", cho phép chuyển sang bất kỳ trạng thái không trống hợp pháp nào.

var phaseOrder = map[Phase]int{
	PhaseInit:     1,
	PhasePremise:  2,
	PhaseOutline:  3,
	PhaseWriting:  4,
	PhaseComplete: 5,
}

// CanTransitionPhase xác định xem Phase có cho phép di chuyển hay không.
// Giữ các quy tắc đơn giản: cho phép di chuyển đồng hình, cho phép chuyển tiếp và không cho phép khôi phục.
func CanTransitionPhase(from, to Phase) bool {
	if to == "" {
		return false
	}
	if from == "" || from == to {
		return true
	}
	fromOrder, fromOK := phaseOrder[from]
	toOrder, toOK := phaseOrder[to]
	if !fromOK || !toOK {
		return false
	}
	return toOrder >= fromOrder
}

// ValidatePhaseTransition xác minh xem quá trình chuyển đổi Giai đoạn có hợp pháp hay không.
func ValidatePhaseTransition(from, to Phase) error {
	if CanTransitionPhase(from, to) {
		return nil
	}
	return fmt.Errorf("invalid phase transition: %q -> %q: %w", from, to, errs.ErrPhaseTransition)
}

// CanTransitionFlow xác định xem FlowState có cho phép di chuyển hay không.
func CanTransitionFlow(from, to FlowState) bool {
	if to == "" {
		return false
	}
	if from == "" || from == to {
		return true
	}

	switch from {
	case FlowWriting:
		return to == FlowReviewing || to == FlowRewriting || to == FlowPolishing || to == FlowSteering
	case FlowReviewing:
		return to == FlowWriting || to == FlowRewriting || to == FlowPolishing || to == FlowSteering
	case FlowRewriting:
		return to == FlowWriting || to == FlowSteering
	case FlowPolishing:
		return to == FlowWriting || to == FlowSteering
	case FlowSteering:
		return to == FlowWriting || to == FlowReviewing || to == FlowRewriting || to == FlowPolishing
	default:
		return false
	}
}

// ValidateFlowTransition xác minh xem quá trình chuyển đổi FlowState có hợp pháp hay không.
func ValidateFlowTransition(from, to FlowState) error {
	if CanTransitionFlow(from, to) {
		return nil
	}
	return fmt.Errorf("invalid flow transition: %q -> %q: %w", from, to, errs.ErrFlowTransition)
}
