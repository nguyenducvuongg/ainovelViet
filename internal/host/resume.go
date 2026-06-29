package host

import (
	"fmt"
	"os"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// buildResumePrompt tạo lời nhắc ngắn và nhãn giao diện người dùng cho Resume dựa trên thực tế.
//
// Hướng dẫn tái cấu trúc (2026-04-20): Tất cả các quyết định "cụ thể nên làm gì tiếp theo" đã được đẩy xuống Bộ định tuyến luồng máy chủ.
// Chức năng này không còn lập kế hoạch hành động cho Điều phối viên nữa mà chỉ thực hiện ba việc:
//  1. Xác định xem có cần khôi phục hay không (Giai đoạn=Hoàn thành hoặc không có Tiến trình → trả về nhãn trống để biểu thị quá trình tạo mới)
//  2. Tạo nhãn phù hợp để hiển thị trong giao diện người dùng (chẳng hạn như "Phục hồi: Đang chờ xem xét kết thúc vòng cung (V2 A3)")
//  3. Chuyển rõ ràng PendingSteer còn lại trong thời gian người dùng ngừng hoạt động cho Điều phối viên
//
// Trả về (lời nhắc, nhãn, lỗi). Nếu nhãn trống nghĩa là không có trạng thái khôi phục (bạn nên tạo nhãn mới).
func buildResumePrompt(store *storepkg.Store) (string, string, error) {
	progress, err := store.Progress.Load()
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	if progress == nil || progress.Phase == domain.PhaseComplete {
		return "", "", nil
	}

	label := describeResume(store, progress)

	var b strings.Builder
	title := progress.NovelName
	if title == "" {
		title = "tiểu thuyết hiện tại"
	}
	b.WriteString(fmt.Sprintf("[Khôi phục] Sách \"%s\"", title))
	if n := len(progress.CompletedChapters); n > 0 {
		b.WriteString(fmt.Sprintf("Đã hoàn thành chương %d", n))
		if progress.TotalChapters > 0 {
			b.WriteString(fmt.Sprintf("(Tổng cộng %d chương)", progress.TotalChapters))
		}
		b.WriteString(fmt.Sprintf(", tổng cộng %d từ", progress.TotalWordCount))
	}
	b.WriteString("。\n")
	b.WriteString("Máy chủ sẽ đưa ra thông báo `[Lệnh phát hành máy chủ]` tiếp theo dựa trên thực tế hiện tại. Thực thi ngay khi nhận được, không điều chỉnh suy luận tiểu thuyết_context trước. \n")

	if meta, _ := store.RunMeta.Load(); meta != nil && meta.PendingSteer != "" {
		b.WriteString("Người dùng \n đã để lại bình luận can thiệp trong thời gian ngừng hoạt động: \n \"")
		b.WriteString(meta.PendingSteer)
		b.WriteString("\"\n trước tiên vui lòng đánh giá và xử lý theo quy định can thiệp của người dùng của Coop.md.")
	}

	return b.String(), label, nil
}

// descriptionResume Tạo nhãn sơ yếu lý lịch mà con người có thể đọc được; không ảnh hưởng đến hành vi của Điều phối viên.
// Tất cả các lộ trình thực thi đều được Flow Router suy luận một cách thực tế; điều này chỉ dành cho "Khôi phục:xxx" của giao diện người dùng.
func describeResume(store *storepkg.Store, progress *domain.Progress) string {
	switch progress.Phase {
	case domain.PhasePremise, domain.PhaseOutline:
		return fmt.Sprintf("Phục hồi: Giai đoạn lập kế hoạch (%s)", progress.Phase)
	case domain.PhaseWriting:
		// Mức độ ưu tiên phù hợp với mức độ ưu tiên ra quyết định của Bộ định tuyến, sao cho nhãn nhất quán với hướng dẫn được gửi đi.
		if pending, _ := store.Signals.LoadPendingCommit(); pending != nil {
			return fmt.Sprintf("Phục hồi: Chương %d Gửi gián đoạn", pending.Chapter)
		}
		if len(progress.PendingRewrites) > 0 {
			verb := "viết lại"
			if progress.Flow == domain.FlowPolishing {
				verb = "đánh bóng"
			}
			return fmt.Sprintf("Khôi phục %s: Chương %d đang chờ xử lý", verb, len(progress.PendingRewrites))
		}
		if progress.Flow == domain.FlowReviewing {
			return "Khôi phục: Đánh giá bị gián đoạn"
		}
		if progress.InProgressChapter > 0 {
			return fmt.Sprintf("Đang khôi phục: Chương %d đang được tiến hành", progress.InProgressChapter)
		}
		if label := describeArcEndLabel(store, progress); label != "" {
			return label
		}
		return fmt.Sprintf("Phục hồi: Tiếp tục từ Chương %d", progress.NextChapter())
	}
	return "hồi phục"
}

// mô tảArcEndLabel tạo nhãn thân thiện với giao diện người dùng cho các trạng thái trung gian khác nhau ở cuối cung/cuối tập.
// Giữ thứ tự tương tự như nhánh cuối của luồng. Lộ trình và đảm bảo rằng nhãn được căn chỉnh theo lệnh đầu tiên của Bộ định tuyến.
func describeArcEndLabel(store *storepkg.Store, progress *domain.Progress) string {
	if !progress.Layered || len(progress.CompletedChapters) == 0 {
		return ""
	}
	lastCh := progress.CompletedChapters[len(progress.CompletedChapters)-1]
	boundary, err := store.Outline.CheckArcBoundary(lastCh)
	if err != nil || boundary == nil || !boundary.IsArcEnd {
		return ""
	}
	vol, arc := boundary.Volume, boundary.Arc
	switch {
	case !store.World.HasArcReview(lastCh):
		return fmt.Sprintf("Khôi phục: Đang chờ xem xét kết thúc hồ sơ (V%d A%d)", vol, arc)
	case !store.Summaries.HasArcSummary(vol, arc):
		return fmt.Sprintf("Khôi phục: Tạo bản tóm tắt đang chờ xử lý (V%d A%d)", vol, arc)
	case boundary.IsVolumeEnd && !store.Summaries.HasVolumeSummary(vol):
		return fmt.Sprintf("Khôi phục: Đang chờ phân tích khối lượng (V%d)", vol)
	case boundary.NeedsExpansion && boundary.NextArc > 0:
		return fmt.Sprintf("Phục hồi: Cung tiếp theo sẽ được mở rộng (V%d A%d)", boundary.NextVolume, boundary.NextArc)
	case boundary.NeedsNewVolume:
		return fmt.Sprintf("Phục hồi: Đang chờ quyết định về tập tiếp theo (cuối V%d)", vol)
	}
	return ""
}
