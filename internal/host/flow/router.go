// Luồng gói thực hiện định tuyến dọc: Máy chủ quyết định tác nhân phụ nào sẽ gọi tiếp theo để thực hiện những gì dựa trên thực tế.
//
// Nguyên tắc thiết kế:
//   - Route là một hàm thuần túy: Trạng thái đầu vào, đầu ra *Hướng dẫn. Không có IO, không có cuộc gọi Store, có thể kiểm tra một lần.
//   - Trạng thái được LoadState xây dựng từ Store bởi LoadState (không tinh khiết) và đọc tất cả các dữ kiện cần thiết để định tuyến cùng một lúc.
//   - Trả về nil là hợp pháp: có nghĩa là "xét xử tình huống và để Điều phối viên LLM tự đưa ra quyết định".
//
// Bộ định tuyến bao gồm việc ra quyết định "bảng tra cứu" (bước tiếp theo trong mỗi chương, xử lý hậu kỳ cuối cung, điều khiển hàng đợi),
// Không bao gồm các quyết định "hiểu ngữ nghĩa" (chọn người lập kế hoạch, xử lý Chỉ đạo người dùng và đưa ra bản tóm tắt).
package flow

import (
	"fmt"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	storepkg "github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// Hướng dẫn hướng dẫn Máy chủ yêu cầu Điều phối viên gọi tác nhân và tác vụ phụ tiếp theo.
type Instruction struct {
	Agent   string // architect_long / architect_short / writer / editor
	Task    string // Mô tả nhiệm vụ cho đại lý phụ
	Reason  string // Lý do hiển thị cho Điều phối viên (tùy chọn, thuận tiện cho việc gỡ lỗi và ghi nhật ký)
	Chapter int    // Số chương liên quan đến nhiệm vụ viết (tiếp tục/viết lại/đánh bóng); 0 có nghĩa là không liên quan (nhiệm vụ biên tập/kiến trúc sư)
}

// Trạng thái là đầu vào của Tuyến: tất cả các dữ kiện phải được khai báo rõ ràng ở đây, không cho phép Tuyến đọc nội bộ Cửa hàng.
type State struct {
	Progress *domain.Progress

	// Chương đã hoàn thành trước đó (cuối Progress.CompletedChapters); 0 có nghĩa là việc viết chưa bắt đầu.
	LastCompleted int

	// Thông tin ranh giới cung từ chương trước; các trường khác là vô nghĩa khi IsArcEnd=false.
	// Phải bằng 0 khi LastCompleted=0 hoặc không ở chế độ Lớp.
	ArcBoundary *storepkg.ArcBoundary

	// Ba sự thật về quá trình xử lý hậu kỳ: phần đánh giá/tóm tắt phần/tóm tắt tập đã được hoàn thành hay chưa.
	HasArcReview     bool
	HasArcSummary    bool
	HasVolumeSummary bool

	// Thiếu các mục trong cài đặt cơ bản (tín hiệu hoàn thành trong giai đoạn lập kế hoạch).
	FoundationMissing []string
}

// Route trả về hướng dẫn tiếp theo dựa trên thực tế; trả về con số không có nghĩa là để Điều phối viên LLM tự quyết định.
//
// Ưu tiên quyết định (loại trừ lẫn nhau, khớp trước từ trên xuống dưới):
//  1. Giai đoạn=Hoàn thành → không (Tóm tắt đầu ra LLM)
//  2. Giai đoạn!=Viết → không (LLM quyết định lựa chọn người lập kế hoạch/hoàn thành kế hoạch)
//  3. PendingRewrites không trống → người viết viết lại/đánh bóng theo hàng đợi
//  4. Flow=Đang xem xét → không (người chỉnh sửa vừa lưu đánh giá và nhánh phán quyết được xử lý bởi lớp công cụ)
//  5. Flow=Chỉ đạo → không (xử lý can thiệp của người dùng)
//  6. Thiếu phần xem lại phần cuối → trình chỉnh sửa (xem lại phần)
//  7. Có bản đánh giá cuối cùng nhưng thiếu phần tóm tắt → trình soạn thảo(tóm tắt phần)
//  8. Có phần tóm tắt cuối tập nhưng thiếu phần tóm tắt tập → editor(tóm tắt tập)
//  9. Cung tiếp theo là bộ xương → architecture_long(expand_arc)
//
// 10. Ở cuối cuốn sách, bạn cần quyết định tập tiếp theo → Architect_long(append_volume / Complete_book)
// 11. Những người khác → nhà văn(write next_chapter)
func Route(s State) *Instruction {
	p := s.Progress
	if p == nil {
		return nil
	}

	// 1. Trạng thái cuối cùng: để LLM xuất bản tóm tắt
	if p.Phase == domain.PhaseComplete {
		return nil
	}

	// 2. Giai đoạn lập kế hoạch do Điều phối viên quyết định (chọn kiến ​​trúc_dài/ngắn + vòng hoàn thiện)
	if p.Phase != domain.PhaseWriting {
		return nil
	}

	// 3. Viết lại/đánh bóng mức độ ưu tiên của hàng đợi (thực tế đã được triển khai ở lớp công cụ và Bộ định tuyến chỉ được gửi đi theo đơn đặt hàng)
	if len(p.PendingRewrites) > 0 {
		ch := p.PendingRewrites[0]
		verb := "viết lại"
		if p.Flow == domain.FlowPolishing {
			verb = "đánh bóng"
		}
		return &Instruction{
			Agent:   "writer",
			Task:    fmt.Sprintf("%s Chương %d", verb, ch),
			Reason:  fmt.Sprintf("Đang chờ xử lý Viết lại Hàng đợi %d chương còn lại", len(p.PendingRewrites)),
			Chapter: ch,
		}
	}

	// 4. Đang được xem xét: save_review vừa được phát hành. Việc nâng cấp/hạ cấp phán quyết được xử lý bởi lớp công cụ và việc định tuyến không can thiệp.
	if p.Flow == domain.FlowReviewing {
		return nil
	}

	// 5. Sự can thiệp của người dùng đang được xử lý: Điều phối viên đang đưa ra quyết định và Máy chủ không giành quyền trước.
	if p.Flow == domain.FlowSteering {
		return nil
	}

	// 6-10. Hoàn thiện hồ quang ở chế độ xếp lớp
	if p.Layered && s.ArcBoundary != nil && s.ArcBoundary.IsArcEnd {
		b := s.ArcBoundary
		switch {
		case !s.HasArcReview:
			return &Instruction{
				Agent:  "editor",
				Task:   fmt.Sprintf("Thực hiện đánh giá cấp độ cung của cung %d, cung %d (scope=arc)", b.Volume, b.Arc),
				Reason: "Đánh giá cuối phần chưa hoàn thành",
			}
		case !s.HasArcSummary:
			return &Instruction{
				Agent:  "editor",
				Task:   fmt.Sprintf("Tạo khối %d khối tóm tắt cung %d (save_arc_summary)", b.Volume, b.Arc),
				Reason: "Tóm tắt Arc chưa hoàn thành",
			}
		case b.IsVolumeEnd && !s.HasVolumeSummary:
			return &Instruction{
				Agent:  "editor",
				Task:   fmt.Sprintf("Tạo bản tóm tắt âm lượng cho âm lượng %d (save_volume_summary)", b.Volume),
				Reason: "Tóm tắt tập chưa hoàn thành",
			}
		case b.NeedsExpansion && b.NextArc > 0:
			return &Instruction{
				Agent:  "architect_long",
				Task:   fmt.Sprintf("Mở rộng âm lượng cung %d %d (loại save_foundation=expand_arc)", b.NextVolume, b.NextArc),
				Reason: "Bộ xương vòng cung tiếp theo sẽ được mở rộng",
			}
		case b.NeedsNewVolume:
			return &Instruction{
				Agent:  "architect_long",
				Task:   "Sau khi đánh giá, gọi save_foundation type=append_volume (viết tiếp) hoặc type=complete_book (kết thúc sách)",
				Reason: "Khi kết thúc tập, bạn phải quyết định nên thêm tập mới hay kết thúc cuốn sách.",
			}
		}
	}

	// 12. Tiếp tục bình thường
	next := p.NextChapter()
	if next <= 0 {
		return nil
	}
	return &Instruction{
		Agent:   "writer",
		Task:    fmt.Sprintf("Viết chương %d", next),
		Reason:  "Tiếp tục đến chương tiếp theo",
		Chapter: next,
	}
}

// FormatMessage định dạng Hướng dẫn thành tin nhắn người dùng gửi đến Điều phối viên.
// Định dạng được cố định để tạo điều kiện thuận lợi cho việc nhận dạng lời nhắc của Điều phối viên và phản hồi trực tiếp LLM.
func FormatMessage(i *Instruction) string {
	return fmt.Sprintf(
		"[Hướng dẫn vấn đề máy chủ] \n Bước tiếp theo: gọi tác nhân phụ (%s, %q) \nagent: %s\ntask: %q\n Lý do: %s\n Đây là hướng dẫn rõ ràng từ lớp quy trình, vui lòng thực hiện lệnh ngay lập tức; các tham số tác nhân/tác vụ của tác nhân phụ phải sử dụng tác nhân/tác vụ trên như hiện tại, không viết lại tác vụ, không điều chỉnh tiểu thuyết_context trước và không đưa ra suy luận trước.",
		i.Agent, i.Task, i.Agent, i.Task, i.Reason,
	)
}
