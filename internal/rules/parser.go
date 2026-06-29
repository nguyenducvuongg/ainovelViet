package rules

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Một tập hợp các trường vấn đề phía trước đã biết được sử dụng để xác định các trường chưa biết và ghi xung đột.
var knownFrontMatterFields = map[string]struct{}{
	"genre":             {},
	"chapter_words":     {},
	"forbidden_chars":   {},
	"forbidden_phrases": {},
	"fatigue_words":     {},
}

// Parse phân tích một bản sao duy nhất của nội dung Rules.md (vấn đề trước + Markdown).
//
// Chiến lược chịu lỗi:
//   - Phân tích cú pháp tổng thể của nội dung phía trước không thành công: không chặn, văn bản chính vẫn được sử dụng làm tùy chọn và xung đột được ghi lại trong Parse_error
//   - Các trường không xác định: bị loại bỏ, xung đột được ghi lại không xác định_field
//   - Lỗi loại trường: loại bỏ trường, xung đột bản ghi type_error
//   - Giá trị trường không hợp lệ (ví dụ: chap_words không thể được phân tích thành một phạm vi): loại bỏ, xung đột bản ghi không hợp lệ
//
// nguồn là đường dẫn tệp, chỉ được sử dụng cho các cuộc xung đột.source; loại xác định mức độ ưu tiên.
func Parse(source string, kind SourceKind, content []byte) Parsed {
	parsed := Parsed{Source: source, Kind: kind}

	fmText, bodyText := splitFrontMatter(content)
	parsed.Preference = strings.TrimSpace(bodyText)

	if strings.TrimSpace(fmText) == "" {
		return parsed
	}

	// Đầu tiên, sắp xếp không theo thứ tự để ánh xạ [chuỗi] bất kỳ, sau đó nhập mạnh trường phân tích cú pháp theo trường.
	// Điều này có thể phân biệt giữa "trường không tồn tại" và "lỗi loại trường" và có thể xác định các trường không xác định.
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(fmText), &raw); err != nil {
		parsed.Conflicts = append(parsed.Conflicts, Conflict{
			Source: source,
			Kind:   ConflictParseError,
			Detail: fmt.Sprintf("vấn đề trước khi phân tích cú pháp YAML không thành công: %v", err),
		})
		return parsed
	}

	for key, val := range raw {
		if _, ok := knownFrontMatterFields[key]; !ok {
			parsed.Conflicts = append(parsed.Conflicts, Conflict{
				Source: source,
				Kind:   ConflictUnknownField,
				Field:  key,
				Detail: fmt.Sprintf("Trường %q không xác định, không được hỗ trợ trong Giai đoạn 1; bỏ qua", key),
			})
			continue
		}
		applyField(&parsed, key, val)
	}

	return parsed
}

// SplitFrontMatter chia tách `---` nội dung được bao bọc phía trước và văn bản nội dung còn lại.
//
// Hiệp định:
//   - Các tập tin bắt đầu bằng `---` (cho phép BOM/dòng trống) được coi là có nội dung phía trước
//   - `---` thứ hai được theo sau bởi văn bản
//   - Không có mặt trước: toàn văn là văn bản chính
//   - Chỉ có phần đầu `---` và không có phần cuối `---`: coi như không có nội dung chính (tránh nuốt toàn bộ văn bản)
func splitFrontMatter(content []byte) (fm, body string) {
	text := string(bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})) // Chuyển đến BOM UTF-8
	lines := strings.Split(text, "\n")

	// Tìm dòng không trống đầu tiên; nếu không phải `---` thì toàn là văn bản
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.TrimSpace(line) == "---" {
			start = i
		}
		break
	}
	if start < 0 {
		return "", text
	}

	// Tìm cái thứ hai `---`
	end := -1
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		// Bắt đầu bằng `---` nhưng chưa đóng: được coi là không quan trọng
		return "", text
	}

	fm = strings.Join(lines[start+1:end], "\n")
	body = strings.Join(lines[end+1:], "\n")
	return fm, body
}

// applyField chèn một trường thô duy nhất vào Parsed.Structured và ghi xung đột khi các loại không khớp.
func applyField(p *Parsed, key string, val any) {
	switch key {
	case "genre":
		s, ok := asString(val)
		if !ok {
			p.Conflicts = append(p.Conflicts, typeErr(p.Source, key, "string", val))
			return
		}
		p.Structured.Genre = strings.TrimSpace(s)

	case "chapter_words":
		rng, ok := parseChapterWords(val)
		if !ok {
			p.Conflicts = append(p.Conflicts, Conflict{
				Source: p.Source,
				Kind:   ConflictInvalidValue,
				Field:  key,
				Detail: fmt.Sprintf("chap_words Khoảng thời gian dự kiến ​​\"min-max\" (ví dụ: 3000-6000) hoặc một giá trị mục tiêu duy nhất (ví dụ: 2500), đã nhận %v", val),
			})
			return
		}
		p.Structured.ChapterWords = rng

	case "forbidden_chars":
		list, ok := asStringList(p, key, val)
		if !ok {
			p.Conflicts = append(p.Conflicts, typeErr(p.Source, key, "[]string", val))
			return
		}
		p.Structured.ForbiddenChars = list

	case "forbidden_phrases":
		list, ok := asStringList(p, key, val)
		if !ok {
			p.Conflicts = append(p.Conflicts, typeErr(p.Source, key, "[]string", val))
			return
		}
		p.Structured.ForbiddenPhrases = list

	case "fatigue_words":
		m, ok := parseFatigueWords(p, val)
		if !ok {
			p.Conflicts = append(p.Conflicts, typeErr(p.Source, key, "map[string]int hoặc []string", val))
			return
		}
		p.Structured.FatigueWords = m
	}
}

// Phạm vi số từ của parsChapterWords là *WordRange và nó chấp nhận ba phương pháp viết:
//   - chuỗi khoảng thời gian "tối thiểu-tối đa" (chẳng hạn như "3000-6000")
//   - ánh xạ {min, max}
//   - Một số nguyên dương N (số trần 2500 hoặc chuỗi "2500") - được hiểu là "Mục tiêu N từ/chương", tự động
//     Mở rộng đến khoảng N±20%. Nếu không, giá trị duy nhất được người dùng viết bằng trực giác sẽ bị âm thầm loại bỏ và quay trở lại giá trị mặc định tích hợp sẵn (vấn đề #41).
func parseChapterWords(val any) (*WordRange, bool) {
	switch v := val.(type) {
	case string:
		s := strings.TrimSpace(v)
		if !strings.Contains(s, "-") { // Viết một giá trị, chẳng hạn như "2500"
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				return wordBandAround(n), true
			}
			return nil, false
		}
		parts := strings.Split(s, "-")
		if len(parts) != 2 {
			return nil, false
		}
		minV, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		maxV, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || minV < 0 || maxV < 0 || minV > maxV {
			return nil, false
		}
		return &WordRange{Min: minV, Max: maxV}, true
	case map[string]any:
		minV, ok1 := asInt(v["min"])
		maxV, ok2 := asInt(v["max"])
		if !ok1 || !ok2 || minV < 0 || maxV < 0 || minV > maxV {
			return nil, false
		}
		return &WordRange{Min: minV, Max: maxV}, true
	default: // Số trần, YAML phân tích thành int/float64
		if n, ok := asInt(v); ok && n > 0 {
			return wordBandAround(n), true
		}
		return nil, false
	}
}

// wordBandAround mở rộng "N từ/chương mục tiêu" thành khoảng thời gian thoải mái là ±20% (chẳng hạn như 2500 → 2000-3000),
// Đặt giá trị ghi đơn lẻ tương đương với một khoảng hợp lý, thay vì một bức tường cứng N-N (một khoảng chặt chẽ sẽ buộc một vòng lặp vô hạn nén).
func wordBandAround(n int) *WordRange {
	return &WordRange{Min: n * 4 / 5, Max: n * 6 / 5}
}

// ParseFatigueWords chấp nhận cả map[string]int (có ngưỡng) và []string (ngưỡng mặc định 1).
//
// Nếu loại khóa đơn bị sai hoặc ngưỡng không hợp lệ, xung đột sẽ được ghi vào p.Conflicts và sẽ không bao giờ bị nuốt chửng trong im lặng.
// Trả về (map, true) chỉ ra rằng có các mục hợp pháp; (nil, false) cho biết loại tổng thể sai hoặc tất cả các mục đều không hợp lệ.
func parseFatigueWords(p *Parsed, val any) (map[string]int, bool) {
	switch v := val.(type) {
	case map[string]any:
		out := make(map[string]int, len(v))
		for k, raw := range v {
			trimmed := strings.TrimSpace(k)
			if trimmed == "" {
				p.Conflicts = append(p.Conflicts, Conflict{
					Source: p.Source,
					Kind:   ConflictInvalidValue,
					Field:  "fatigue_words",
					Detail: "mệt mỏi_words có một khóa trống và đã bị bỏ qua",
				})
				continue
			}
			n, ok := asInt(raw)
			if !ok {
				p.Conflicts = append(p.Conflicts, Conflict{
					Source: p.Source,
					Kind:   ConflictTypeError,
					Field:  "fatigue_words." + trimmed,
					Detail: fmt.Sprintf("mệt_words[%q] ngưỡng int dự kiến, đã nhận %T (%v); chìa khóa đã bị loại bỏ", trimmed, raw, raw),
				})
				continue
			}
			if n <= 0 {
				p.Conflicts = append(p.Conflicts, Conflict{
					Source: p.Source,
					Kind:   ConflictInvalidValue,
					Field:  "fatigue_words." + trimmed,
					Detail: fmt.Sprintf("Ngưỡng Fatigue_words[%q] phải > 0, đã nhận được %d; chìa khóa đã bị loại bỏ", trimmed, n),
				})
				continue
			}
			out[trimmed] = n
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	case []any:
		out := make(map[string]int, len(v))
		for i, raw := range v {
			s, ok := raw.(string)
			if !ok {
				p.Conflicts = append(p.Conflicts, Conflict{
					Source: p.Source,
					Kind:   ConflictTypeError,
					Field:  fmt.Sprintf("fatigue_words[%d]", i),
					Detail: fmt.Sprintf("phần tử danh sách mệt mỏi_words chuỗi mong đợi, đã nhận %T (%v); phần tử bị loại bỏ", raw, raw),
				})
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				p.Conflicts = append(p.Conflicts, Conflict{
					Source: p.Source,
					Kind:   ConflictInvalidValue,
					Field:  fmt.Sprintf("fatigue_words[%d]", i),
					Detail: "phần tử danh sách mệt mỏi_words trống; bỏ đi",
				})
				continue
			}
			out[s] = 1
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

// asString / asInt / asStringList là các công cụ nhỏ để chuẩn hóa kiểu sau khi khử lưu lượng yaml.v3.
//
// Chiến lược nghiêm ngặt (Debug-First): chỉ chấp nhận loại mục tiêu và không tự động chuyển đổi các loại khác.
// Lỗi loại do người gọi ghi vào do xung đột và không được sửa một cách thầm lặng trong công cụ.

// asString chỉ chấp nhận chuỗi vô hướng.
// Lưu ý: `genre: 42` (không có dấu ngoặc kép) trong YAML sẽ được giải tuần tự hóa thành int, được xác định là lỗi kiểu theo hàm này.
// Người dùng nên viết `genre: "42"` để khai báo rõ ràng chuỗi.
func asString(v any) (string, bool) {
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

// asInt chấp nhận tất cả các loại số nguyên; float64 chỉ được chấp nhận nếu nó là số nguyên (số YAML phân tích thành float64 theo mặc định).
// Số chuỗi không còn được chuyển đổi tự động nữa - để tránh nhầm lẫn với lỗi "đặt sai trường thành chuỗi".
func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		// Chỉ được chấp nhận nếu float là số nguyên (vì yaml được phân tích cú pháp `5` → float64(5.0))
		if x == float64(int(x)) {
			return int(x), true
		}
		return 0, false
	default:
		return 0, false
	}
}

// Các phần tử asStringList phải là chuỗi; nếu không thì phần tử sẽ bị bỏ qua và xung đột được viết ra.
// Trả về (danh sách, đúng) chỉ ra rằng có các yếu tố pháp lý; (nil, false) chỉ ra rằng loại tổng thể sai hoặc tất cả các phần tử đều không hợp lệ.
func asStringList(p *Parsed, field string, v any) ([]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for i, raw := range arr {
		s, ok := raw.(string)
		if !ok {
			p.Conflicts = append(p.Conflicts, Conflict{
				Source: p.Source,
				Kind:   ConflictTypeError,
				Field:  fmt.Sprintf("%s[%d]", field, i),
				Detail: fmt.Sprintf("Chuỗi mong đợi phần tử danh sách %s, đã nhận được %T (%v); phần tử đã bị loại bỏ", field, raw, raw),
			})
			continue
		}
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func typeErr(source, field, expected string, got any) Conflict {
	return Conflict{
		Source: source,
		Kind:   ConflictTypeError,
		Field:  field,
		Detail: fmt.Sprintf("Trường %s sai loại, dự kiến ​​%s, nhận %T (%v); bỏ đi", field, expected, got, got),
	}
}
