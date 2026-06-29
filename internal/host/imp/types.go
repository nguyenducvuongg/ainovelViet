// Gói imp thực hiện việc nhập và suy luận ngược của các chương mới bên ngoài.
//
// Ý tưởng cốt lõi: Sử dụng LLM để đảo ngược nền tảng + thông tin thực tế của từng chương và sử dụng lại save_foundation / hiện có
// Bộ ba phần nguyên tử của công cụ commit_chapter được đặt trên đĩa. Sau khi nhập xong, trạng thái cửa hàng tương đương với "N chương đã được viết.
// "Sau sự cố", người gọi có thể gọi Host.Resume() để tiếp tục viết liền mạch.
//
// Không sử dụng Điều phối viên: Nhập là phát lại mang tính quyết định và không thuộc phạm vi ra quyết định LLM; để điều phối viên
// Việc can thiệp sẽ chỉ gây ra sự không chắc chắn. Gói này điều chỉnh trực tiếp máy khách LLM + công cụ điều chỉnh.
package imp

import "time"

// Chương là một chương được chia duy nhất.
type Chapter struct {
	Title   string
	Content string
}

// Tùy chọn kiểm soát hành vi nhập.
type Options struct {
	// SourcePath là bắt buộc. Đường dẫn tệp txt/md đơn.
	SourcePath string

	// Tiếp tục Từ Tùy chọn. Nhập khẩu bắt đầu từ chương N; 0/1 nghĩa là bắt đầu lại từ đầu.
	// Nếu > 1, việc đẩy ngược của Foundation sẽ bị bỏ qua (sẽ coi như lệnh đã được đặt).
	ResumeFrom int
}

// Giai đoạn thể hiện giai đoạn hiện tại của quá trình nhập khẩu.
type Stage string

const (
	StageSplitting  Stage = "splitting"
	StageFoundation Stage = "foundation"
	StageChapter    Stage = "chapter"
	StageDone       Stage = "done"
	StageError      Stage = "error"
)

// Sự kiện là sự kiện tiến trình do quá trình nhập phát ra.
type Event struct {
	Time    time.Time
	Stage   Stage
	Current int    // chương Số chương hiện tại của giai đoạn; 0 cho các giai đoạn khác
	Total   int    // Tổng số chương
	Message string // mô tả con người có thể đọc được
	Err     error  // StageError được mang khi
}
