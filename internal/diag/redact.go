package diag

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/store"
)

// SkelEvent là bộ khung hành vi được giải mẫn cảm của thông báo phiên: giữ lại các tín hiệu cấu trúc (vai trò/công cụ/lỗi/
// Dấu vân tay trùng lặp), tất cả văn bản miễn phí (văn bản, lời nhắc, suy nghĩ) sẽ được mã hóa. Điều này tốt hơn
// store.compactMessage là lớp chiếu chặt chẽ hơn - lớp sau được nén theo âm lượng (>4KB) và âm lượng không được xem xét ở đây.
// Không có văn bản đi ra khỏi gói.
type SkelEvent struct {
	Agent    string     // Phiên nguồn: điều phối viên/người viết-ch07…
	Role     string     // assistant / tool / user
	Tools    []SkelTool // Cuộc gọi công cụ trong tin nhắn này
	ErrClass string     // role=tool và is_error: dòng đầu tiên của lỗi (chuỗi lỗi khung, không bao gồm văn bản)
	TextSha  string     // Hàm băm ngắn của văn bản được mã hóa; giống như sha = tạo ra cùng một phân đoạn nhiều lần (tín hiệu tuần hoàn)
	Redacted int        // Số lượng văn bản/khối suy nghĩ được mã hóa trong bài viết này (được sử dụng để tự kiểm tra giải mẫn cảm)
}

// SkelTool là một phép chiếu được giải mẫn cảm của lệnh gọi công cụ.
type SkelTool struct {
	Name     string            // Tên công cụ (tín hiệu có cấu trúc, không có văn bản)
	Args     map[string]string // khóa → giá trị gốc vô hướng / chuỗi ngắn có dấu ngoặc kép / "<len sha được điều chỉnh lại>"
	Invalid  bool              // ArgsInvalid: Không thể phân tích cú pháp các tham số do mô hình gửi (tín hiệu #34)
	ParseErr string            // ArgsParseError: Lý do phân tích cú pháp thất bại
}

// redactMessage chiếu một Agentcore.Message vào một khung hành vi.
func redactMessage(agent string, m agentcore.Message) SkelEvent {
	ev := SkelEvent{Agent: agent, Role: string(m.Role)}
	isErr, _ := m.Metadata["is_error"].(bool)

	var text strings.Builder
	for _, b := range m.Content {
		switch b.Type {
		case agentcore.ContentText:
			// Kết quả lỗi tool giữ nguyên dòng đầu tiên: đây là chuỗi lỗi của chính chúng ta (chẳng hạn như inputValidationError),
			// Nó không chứa văn bản và là chìa khóa để định vị vòng lặp. Tất cả các văn bản khác sẽ được nhập vào nhóm mã hóa.
			if m.Role == agentcore.RoleTool && isErr && ev.ErrClass == "" {
				ev.ErrClass = firstLine(b.Text, 160)
				continue
			}
			if strings.TrimSpace(b.Text) != "" {
				text.WriteString(b.Text)
				ev.Redacted++
			}
		case agentcore.ContentThinking:
			if strings.TrimSpace(b.Thinking) != "" {
				text.WriteString(b.Thinking)
				ev.Redacted++
			}
		case agentcore.ContentToolCall:
			if b.ToolCall != nil {
				ev.Tools = append(ev.Tools, redactToolCall(b.ToolCall))
			}
		}
	}
	if t := text.String(); t != "" {
		ev.TextSha = shortHash(t)
	}
	return ev
}

// redactToolCall chiếu lệnh gọi công cụ: tên công cụ + tham số (giải mẫn cảm giá trị) + cờ ngoại lệ phân tích cú pháp.
func redactToolCall(tc *agentcore.ToolCall) SkelTool {
	return SkelTool{
		Name:     tc.Name,
		Args:     redactArgs(tc.Args),
		Invalid:  tc.ArgsInvalid,
		ParseErr: tc.ArgsParseError,
	}
}

// redactArgs chiếu các đối tượng tham số công cụ vào khóa → giá trị được xử lý lại. Tham số phi đối tượng trả về con số không
// (ArgsInvalid/ParseErr đã được ghi lại riêng trong SkelTool).
func redactArgs(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = projectValue(v)
	}
	return out
}

// projectValue chiếu một giá trị tham số duy nhất dưới dạng loại JSON:
//   - Scalar (number/bool/null): giá trị ban đầu là tín hiệu cấu trúc, được giữ lại (chương: 7)
//   - Chuỗi định danh ngắn: dành riêng bằng dấu ngoặc kép, loại hiển thị (chương: tín hiệu số được xâu chuỗi của "7" ← # 34)
//   - Các chuỗi, đối tượng và mảng chứa văn bản tiếng Trung/dấu cách/dài: được mã hóa là <redacted…> (không có văn bản nào ngoài gói)
//   - Đã là trình giữ chỗ [session_compact: …]: an toàn và nhiều thông tin, hãy giữ nguyên như vậy
func projectValue(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return ""
	}
	switch s[0] {
	case '"':
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return redactPlaceholder(s)
		}
		if strings.HasPrefix(str, store.CompactTag) {
			return str
		}
		// Chỉ giữ các giá trị ngắn "như định danh/số/liệt kê" (chương: "7", type: "tiền đề", tác nhân: "người viết");
		// Bất kỳ chuỗi nào chứa ký tự tiếng Trung, dấu cách hoặc các ký hiệu khác sẽ được coi là văn bản và sẽ được mã hóa.
		if utf8.RuneCountInString(str) <= 32 && isStructuralToken(str) {
			return strconv.Quote(str)
		}
		return redactPlaceholder(str)
	case '{':
		return fmt.Sprintf("<redacted object len=%d>", len(raw))
	case '[':
		return fmt.Sprintf("<redacted array len=%d>", len(raw))
	default:
		return s
	}
}

// isStructuralToken xác định xem một chuỗi có "giống như mã định danh" hay không - các chữ cái/số thuần ASCII/`_-.:/`,
// Không có khoảng trống, không có tiếng Trung Quốc. Dùng để phân biệt tín hiệu cấu trúc (được giữ lại) với các đoạn văn bản (được mã hóa).
func isStructuralToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == ':' || r == '/':
		default:
			return false
		}
	}
	return true
}

func redactPlaceholder(s string) string {
	return fmt.Sprintf("<redacted len=%d sha=%s>", utf8.RuneCountInString(s), shortHash(s))
}

// shortHash lấy hàm băm ngắn của văn bản; nó chỉ được sử dụng để xác định "liệu văn bản giống nhau có xuất hiện lặp lại hay không" và không được sử dụng để mã hóa.
func shortHash(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}

// firstLine lấy dòng đầu tiên và cắt bớt nó bằng rune để cung cấp bản tóm tắt về chuỗi lỗi.
func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max]) + "…"
	}
	return s
}
