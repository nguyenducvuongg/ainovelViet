package imp

import (
	"fmt"
	"regexp"
	"strings"
)

// phong bìTagRe khớp với === TAG === dòng (khoảng trắng tùy chọn trước và sau), không phân biệt chữ hoa chữ thường.
var envelopeTagRe = regexp.MustCompile(`(?m)^\s*===\s*([A-Z_]+)\s*===\s*$`)

// ParseTaggedEnvelope phân tích đầu ra nhiều phân đoạn dưới dạng `=== TAG ===\nbody...` vào bản đồ.
// Khóa là tên thẻ viết hoa và giá trị là đoạn văn tương ứng (khoảng trống đầu tiên và cuối cùng đã được cắt bớt).
// Khi các thẻ trùng lặp xảy ra, thẻ sau sẽ ghi đè lên thẻ trước.
func parseTaggedEnvelope(text string) map[string]string {
	matches := envelopeTagRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make(map[string]string, len(matches))
	for i, m := range matches {
		tag := strings.ToUpper(text[m[2]:m[3]])
		bodyStart := m[1]
		bodyEnd := len(text)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		out[tag] = strings.TrimSpace(text[bodyStart:bodyEnd])
	}
	return out
}

// requireTags kiểm tra xem phong bì phải chứa thẻ đã cho và không trống.
func requireTags(env map[string]string, tags ...string) error {
	var missing []string
	for _, t := range tags {
		if strings.TrimSpace(env[t]) == "" {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tags: %s", strings.Join(missing, ", "))
	}
	return nil
}
