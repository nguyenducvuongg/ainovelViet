// Gói exp triển khai khả năng xuất các chương đã hoàn thành.
//
// Đối xứng với imp/: IO cục bộ thuần túy, không dựa vào LLM, không thay đổi trạng thái cửa hàng. Xuất khẩu có thể được thực hiện với
// Điều phối viên chạy đồng thời (Tiến trình chỉ đọc + bản nháp cuối cùng của chương), là khả năng theo chiều ngang.
//
// Phiên bản đầu tiên chỉ hỗ trợ TXT; EPUB còn lại cho vòng tiếp theo.
package exp

import "github.com/nguyenducvuongg/ainovelViet/internal/store"

// Định dạng xác định định dạng xuất.
type Format string

const (
	// FormatTXT đầu ra văn bản thuần túy.
	FormatTXT Format = "txt"
	// FormatEPUB Bộ chứa EPUB 3 tiêu chuẩn (zip + xhtml).
	FormatEPUB Format = "epub"
)

// Tùy chọn kiểm soát hành vi xuất. giá trị 0 tương đương với "Xuất toàn bộ tệp sang đường dẫn mặc định và sẽ báo lỗi nếu tệp tồn tại."
//
// Định dạng: "Tên sách" → Tách tập → Văn bản chương. Hai loại dữ liệu nội bộ không thể được nhập hoặc xuất: tiền đề (bản thiết kế sáng tạo,
// Chứa thông tin meta nền như trình đọc mục tiêu/điểm tiêu thụ cốt lõi/khu vực hạn chế viết, để tác giả và công cụ xem, không phải cho người đọc);
// Tách vòng cung (cung là cấu trúc bên trong quá mỏng theo quan điểm của người đọc). Tiêu đề sách và phần tách tập luôn được giữ nguyên.
type Options struct {
	// Khi Định dạng là một chuỗi trống, nó được suy ra từ hậu tố OutPath (.txt → TXT, .epub → EPUB);
	// Dự phòng về FormatTXT khi OutPath cũng trống. Người gọi SDK có thể chỉ định rõ ràng để bỏ qua suy luận.
	Format Format

	// Đường dẫn tệp đầu ra OutPath; trống có nghĩa là {novelDir}/{NovelName}.{ext},
	// ext được xác định bởi Định dạng (nếu NovelName trống, tên thư mục sẽ được sử dụng).
	OutPath string

	// Từ / Đến phạm vi chương, khoảng thời gian đóng. 0 nghĩa là từ chương 1/ đến chương cuối cùng.
	// Các chương chưa hoàn thành trong phạm vi sẽ được bỏ qua và ghi vào Result.Skipped và không bị coi là lỗi.
	From, To int

	// Ghi đè Có ghi đè lên tệp nếu nó tồn tại hay không; bị từ chối theo mặc định.
	Overwrite bool
}

// Deps là các phần phụ thuộc mà Run yêu cầu. Chỉ lưu trữ; không cần LLM, lời nhắc hoặc gói để xuất.
type Deps struct {
	Store *store.Store
}

// Kết quả là bảng tổng hợp sản phẩm xuất khẩu thành công.
type Result struct {
	// Đường dẫn Đường dẫn tệp thực tế được ghi (tuyệt đối hoặc tương đối do người gọi truyền vào).
	Path string
	// Số chương Số chương thực sự được viết.
	Chapters int
	// Byte Số byte tệp (UTF-8).
	Bytes int
	// Bỏ qua Số chương nằm trong phạm vi được yêu cầu nhưng chưa hoàn thành.
	Skipped []int
}
