package utils

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// DecodeText giải mã byte tệp văn bản do người dùng cung cấp thành UTF-8: UTF-8 bất hợp pháp theo GB18030
//(GBK Superset) Chuyển mã - Một số lượng lớn các văn bản tiểu thuyết Trung Quốc lưu hành trên Internet được mã hóa bằng GBK và đọc trực tiếp dưới dạng UTF-8
// Tất cả chỉ là vô nghĩa. Các chuỗi byte không phải GBK sẽ được bộ giải mã thay thế bằng U+FFFD (là mã bị cắt xén và được xác định bởi người gọi
// Không có lượt truy cập, báo cáo lỗi để hướng dẫn người dùng). Cuối cùng loại bỏ BOM UTF-8 (nếu không trận đấu đầu tiên sẽ mang theo nó).
func DecodeText(data []byte) string {
	if !utf8.Valid(data) {
		if decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data); err == nil {
			data = decoded
		}
	}
	return strings.TrimPrefix(string(data), "\uFEFF")
}
