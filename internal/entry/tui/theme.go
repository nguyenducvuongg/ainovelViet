package tui

import "github.com/charmbracelet/lipgloss"

// Bảng màu chủ đề—Phong cách sách ấm áp
// AdaptiveColor: Light = giá trị màu nền sáng, Dark = giá trị màu nền tối
//
// Nguyên tắc thiết kế: Ánh sáng ổn định ở số 1 (đáy sáng đã được điều chỉnh đạt hiệu quả vừa ý); Độ tối cao hơn ánh sáng đồng đều ở cấp số một
// Tăng độ sáng ~25% và tăng độ bão hòa một chút để đảm bảo đủ độ tương phản trong nền tối (trước colorDim #6b6355
// Hầu như không hiển thị trên nền đen #1c1c1c, các dấu chia/văn bản phụ đều biến mất).
//
// Nền tối của colorAccent2 được đổi từ #7a9e7e thành xanh #5fb8a3, tương tự như "xanh khỏe mạnh" của colorSuccess
// On - trước đây cả hai đều có màu sắc giống hệt nhau, gây nhầm lẫn giữa thang màu của đặc vụ kiến ​​​​trúc sư và niềm vui "hit cao".
// bodyTextColor là chiến lược tiền cảnh cho "nội dung trung tính":
//   - Thiết bị đầu cuối tối → NoColor, kế thừa nền trước mặc định của thiết bị đầu cuối, để ngăn chúng tôi buộc người dùng phải định cấu hình #e8e0d0 Mibai
//     Màu sắc tương phản trên chủ đề ấm/lạnh (người dùng đã kiểm tra rằng màu mặc định trên nền tối dễ đọc hơn).
//   - Thiết bị đầu cuối sáng → Sử dụng tệp Light của colorText (màu nâu đậm #3d3529) để giữ tông màu ấm của thương hiệu;
//     Độ tương phản màu đen mặc định trên nền sáng quá cứng và màu nâu sẫm được điều chỉnh ban đầu trông mềm mại hơn trên nền sáng.
//
// AdaptiveColor phải đưa ra giá trị màu ở cả hai đầu. Không có tệp "không màu" nên nền được đánh giá một lần khi khởi động.
// Sau đó, tất cả "văn bản trung lập" chẳng hạn như giá trị tổng quan/văn bản chương/mô tả lệnh sẽ tham chiếu thống nhất bodyTextColor.
var bodyTextColor lipgloss.TerminalColor = func() lipgloss.TerminalColor {
	if lipgloss.HasDarkBackground() {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color("#3d3529")
}()

var (
	colorText    = lipgloss.AdaptiveColor{Light: "#3d3529", Dark: "#e8e0d0"}
	colorDim     = lipgloss.AdaptiveColor{Light: "#8a7e6b", Dark: "#8a8175"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#7a7060", Dark: "#b8b09c"}
	colorAccent  = lipgloss.AdaptiveColor{Light: "#b8860b", Dark: "#e5b449"}
	colorAccent2 = lipgloss.AdaptiveColor{Light: "#3d7a42", Dark: "#5fb8a3"}
	colorRunning = lipgloss.AdaptiveColor{Light: "#6f8641", Dark: "#b5d075"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#3d7a42", Dark: "#7ec488"}
	colorError   = lipgloss.AdaptiveColor{Light: "#b5433a", Dark: "#e07060"}
	colorReview  = lipgloss.AdaptiveColor{Light: "#b07530", Dark: "#e09b5a"}
	colorContext = lipgloss.AdaptiveColor{Light: "#6b5a9e", Dark: "#a890d8"}
	colorTool    = lipgloss.AdaptiveColor{Light: "#3a7a8a", Dark: "#7ec5d8"}
)

// Ánh xạ màu nhãn trạng thái
var statusColors = map[string]lipgloss.AdaptiveColor{
	"READY":    colorDim,
	"PAUSING":  colorAccent,
	"PAUSED":   colorAccent,
	"RUNNING":  colorRunning,
	"REVIEW":   colorReview,
	"REWRITE":  colorReview,
	"COMPLETE": colorSuccess,
	"ERROR":    colorError,
}

// Hiển thị trạng thái: biểu tượng + nhãn tiếng Trung. Phù hợp với chủ đề tổng thể ấm áp, tránh các khối màu đồng nhất mang tính đột ngột.
// Biểu tượng CHẠY được để trống và được khung quay lấp đầy động, cho phép cảm giác động được tích hợp vào chính chỉ báo trạng thái.
var statusDisplay = map[string]struct {
	icon  string
	label string
}{
	"READY":    {"○", "sẵn sàng"},
	"RUNNING":  {"", "Đang chạy"},
	"REVIEW":   {"◆", "Ôn tập"},
	"REWRITE":  {"◆", "Làm lại"},
	"COMPLETE": {"●", "Hoàn thành"},
	"PAUSED":   {"⏸", "tạm dừng"},
	"PAUSING":  {"⏸", "Đã tạm dừng"},
	"ERROR":    {"✕", "sai lầm"},
}

// Ánh xạ màu phân loại sự kiện
var categoryColors = map[string]lipgloss.AdaptiveColor{
	"DISPATCH": colorAccent,
	"DONE":     colorSuccess,
	"TOOL":     colorTool,
	"SYSTEM":   colorAccent,
	"USER":     colorAccent2,
	"REVIEW":   colorReview,
	"CHECK":    colorSuccess,
	"ERROR":    colorError,
	"AGENT":    colorMuted,
	"CONTEXT":  colorContext,
	"COMPACT":  colorContext,
}

// Phong cách cơ bản
var (
	baseBorder = lipgloss.RoundedBorder()

	topBarStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(0, 1)

	statusIconStyle = lipgloss.NewStyle().
			Bold(true)

	statusLabelStyle = lipgloss.NewStyle().
				Foreground(colorText)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	fieldLabelStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(10)

	// fieldValueStyle / cardContentStyle sử dụng bodyTextColor - giá trị của vùng tổng quan (trạng thái chạy,
	// Số chương đã hoàn thành, số từ, v.v.), mục dàn ý, danh sách ký tự, tóm tắt chương, v.v. "nội dung văn bản trung tính"
	// Trên nền tối, hãy làm theo màu nền trước mặc định của thiết bị đầu cuối (để tránh buộc màu trắng xung đột với chủ đề), còn trên nền sáng, hãy sử dụng màu nâu sẫm để giữ tông màu ấm.
	// Các phần tử có ngữ nghĩa mạnh (tiêu đề, giá trị đánh dấu, trạng thái, lỗi, tô màu tỷ lệ truy cập, v.v.) vẫn sử dụng colorAccent /
	// colorError và các màu chủ đề khác.
	fieldValueStyle = lipgloss.NewStyle().Foreground(bodyTextColor)

	highlightValueStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	contextUsageMetaStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	cardTitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	cardContentStyle = lipgloss.NewStyle().Foreground(bodyTextColor)
)
