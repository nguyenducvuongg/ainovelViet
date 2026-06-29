package utils

import (
	"strings"
	"unicode"
)

// CleanInputText loại bỏ các ký tự điều khiển không có ý nghĩa kinh doanh trong dữ liệu đầu vào của thiết bị đầu cuối và giữ lại văn bản mà người dùng có thể nhìn thấy.
// Trong trường hợp nhập một dòng, dòng mới và tab trong văn bản được dán sẽ được chuẩn hóa thành khoảng trắng.
func CleanInputText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// CleanInputLine làm sạch một dòng đầu vào thủ công và loại bỏ các khoảng trống ở đầu và cuối.
func CleanInputLine(s string) string {
	return strings.TrimSpace(CleanInputText(s))
}

func CleanInputRunes(runes []rune) string {
	var b strings.Builder
	for _, r := range runes {
		if r == '\n' || r == '\r' || r == '\t' {
			b.WriteByte(' ')
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func ContainsControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
