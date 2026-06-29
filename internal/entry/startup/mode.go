package startup

import "fmt"

// Lớp khởi động lưu trữ quá trình điều phối khởi động "trước khi vào Engine".
// Quy ước phân cấp:
// 1. lối vào/tui và lối vào/không đầu là lối vào chủ nhà;
// 2. Công ty khởi nghiệp chịu trách nhiệm về các chiến lược khởi nghiệp như nhanh chóng/đồng sáng tạo/tiếp tục;
// 3. Orchestrator.Engine chỉ chịu trách nhiệm thực hiện phiên chính thức và không chịu trách nhiệm chuẩn bị trước chế độ.

// Chế độ đại diện cho loại chính sách khởi động trước khi vào Engine.
type Mode string

const (
	// ModeQuick sử dụng trực tiếp thông tin đầu vào của người dùng làm điểm bắt đầu để tạo.
	ModeQuick Mode = "quick"
	// Trước tiên, ModeCoCreate thực hiện nhiều vòng làm rõ, sau đó tạo bản nháp quảng cáo vào Engine.
	ModeCoCreate Mode = "cocreate"
	// ModeContinueFromNovel tập hợp bối cảnh để tiếp tục viết dựa trên nội dung tiểu thuyết hiện có.
	ModeContinueFromNovel Mode = "continue_from_novel"
)

// Yêu cầu mô tả đầu vào ban đầu được gửi bởi lớp đầu vào tới lớp chiến lược khởi động.
// Cổng máy chủ trước tiên thu thập thông tin đầu vào của người dùng, sau đó quá trình khởi động sẽ sắp xếp nó thành một kế hoạch có thể vào Engine.
type Request struct {
	Mode        Mode
	UserPrompt  string
	NovelPath   string
	OutputDir   string
	Interactive bool
}

// Kế hoạch mô tả kết quả được tạo ra bởi lớp chiến lược khởi động.
// Lối vào máy chủ không được tự ghép nối lời nhắc khởi động chính thức mà phải sử dụng Kế hoạch rồi điều khiển Động cơ.
type Plan struct {
	Mode        Mode
	DisplayName string
	StartPrompt string
	ResumeOnly  bool
}

// Chính sách giữ chỗ đánh dấu ErrNotImplemented chưa được triển khai.
var ErrNotImplemented = fmt.Errorf("startup mode not implemented")

// Chuẩn bịTiếp tụcFromNovel là một điểm đặt trước thống nhất để "tiếp tục dựa trên các tiểu thuyết hiện có".
// Trong tương lai, TUI/headless trước tiên phải sắp xếp dữ liệu đầu vào thành Yêu cầu, sau đó tạo Kế hoạch từ đây để có thể nhập vào Công cụ.
func PrepareContinueFromNovel(req Request) (Plan, error) {
	return Plan{}, fmt.Errorf("%w: %s", ErrNotImplemented, ModeContinueFromNovel)
}
