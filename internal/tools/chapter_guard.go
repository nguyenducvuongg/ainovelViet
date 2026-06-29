package tools

import (
	"fmt"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/errs"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// EnsureChapterExpanded verifies that a chapter is inside the currently
// expanded layered outline. Non-layered books and non-writing phases have no
// layered range constraint.
func EnsureChapterExpanded(st *store.Store, chapter int) error {
	if st == nil || chapter <= 0 {
		return nil
	}
	progress, err := st.Progress.Load()
	if err != nil {
		return fmt.Errorf("load progress: %w: %w", errs.ErrStoreRead, err)
	}
	if progress == nil || !progress.Layered || progress.Phase != domain.PhaseWriting {
		return nil
	}
	boundary, err := st.Outline.CheckArcBoundary(chapter)
	if err != nil {
		return fmt.Errorf("check layered outline: %w: %w", errs.ErrStoreRead, err)
	}
	if boundary != nil {
		return nil
	}
	return fmt.Errorf(
		"Chương %d không nằm trong phạm vi đề cương phân cấp: viết trước tiên phải mở rộng_arc hoặc nối thêm_volume để thêm tập; nếu cuốn sách đã hoàn thành, vui lòng gọi save_foundation type=complete_book: %w",
		chapter, errs.ErrToolPrecondition)
}
