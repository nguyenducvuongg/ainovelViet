package rules

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Hợp nhất kết hợp nhiều nguồn được trình tải trả về vào Gói cuối cùng.
//
// Hợp nhất các quy tắc:
//   - Các trường có cấu trúc thông thường: mức độ ưu tiên gần nhất (cái sau ghi đè cái trước), nhiều nguồn khai báo cùng một trường và các giá trị không nhất quán, ghi field_conflict
//   - mệt_words: ghép theo từ; khi cùng một từ được khai báo từ nhiều nguồn và các ngưỡng không nhất quán, mức độ ưu tiên gần nhất sẽ được đưa ra và field_conflict được ghi.
//   - Markdown text: Ghép theo thứ tự nguồn, thêm tiêu đề nguồn vào từng đoạn văn, không ghi đè
//   - nguồn: tất cả các đường dẫn tệp được tải thành công
//   - xung đột: xung đột giai đoạn phân tích + giai đoạn hợp nhất field_conflict
//
// Các lớp tham số đầu vào phải được sắp xếp theo thứ tự tăng dần theo SourceKind (dạng đầu ra của Loader.Load).
func Merge(layers []Parsed) Bundle {
	bundle := Bundle{
		Structured:  Structured{},
		Preferences: "",
		Sources:     make([]string, 0, len(layers)),
		Conflicts:   nil,
	}

	// Giai đoạn A: Thu thập tất cả các nguồn khai báo của từng trường để tạo điều kiện thuận lợi cho việc xác định xung đột tiếp theo
	declarations := map[string][]Parsed{}
	declare := func(field string, p Parsed) {
		declarations[field] = append(declarations[field], p)
	}
	for _, p := range layers {
		if p.Structured.Genre != "" {
			declare("genre", p)
		}
		if p.Structured.ChapterWords != nil {
			declare("chapter_words", p)
		}
		if len(p.Structured.ForbiddenChars) > 0 {
			declare("forbidden_chars", p)
		}
		if len(p.Structured.ForbiddenPhrases) > 0 {
			declare("forbidden_phrases", p)
		}
		if len(p.Structured.FatigueWords) > 0 {
			declare("fatigue_words", p)
		}
	}

	// Giai đoạn B: Hợp nhất các trường có cấu trúc để thu được các trường có cấu trúc cuối cùng.
	// Trường vô hướng/danh sách duy trì phạm vi bao phủ gần đó; mệt mỏi_words là một bản đồ được xếp chồng lên nhau bởi các từ, giúp người dùng chỉ cần thêm một số lượng nhỏ các từ mệt mỏi.
	for _, p := range layers {
		if p.Structured.Genre != "" {
			bundle.Structured.Genre = p.Structured.Genre
		}
		if p.Structured.ChapterWords != nil {
			bundle.Structured.ChapterWords = p.Structured.ChapterWords
		}
		if len(p.Structured.ForbiddenChars) > 0 {
			bundle.Structured.ForbiddenChars = p.Structured.ForbiddenChars
		}
		if len(p.Structured.ForbiddenPhrases) > 0 {
			bundle.Structured.ForbiddenPhrases = p.Structured.ForbiddenPhrases
		}
		if len(p.Structured.FatigueWords) > 0 {
			bundle.Structured.FatigueWords = mergeFatigueWords(bundle.Structured.FatigueWords, p.Structured.FatigueWords)
		}
	}

	// Giai đoạn C: Xây dựng field_conflict (nhiều nguồn + giá trị không nhất quán được coi là xung đột)
	for field, sources := range declarations {
		if len(sources) < 2 {
			continue
		}
		if field == "fatigue_words" {
			bundle.Conflicts = append(bundle.Conflicts, fatigueWordConflicts(sources)...)
			continue
		}
		if allEqual(field, sources) {
			continue
		}
		bundle.Conflicts = append(bundle.Conflicts, Conflict{
			Source: sources[len(sources)-1].Source,
			Kind:   ConflictFieldConflict,
			Field:  field,
			Detail: describeFieldConflict(field, sources),
		})
	}

	// Giai đoạn D: Hợp nhất nội dung tùy chọn Markdown
	var sb strings.Builder
	for _, p := range layers {
		if strings.TrimSpace(p.Preference) == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "## [%s] %s\n\n", p.Kind, p.Source)
		sb.WriteString(p.Preference)
	}
	bundle.Preferences = sb.String()

	// Giai đoạn E: Tổng hợp các nguồn và xung đột phân tích cú pháp
	for _, p := range layers {
		bundle.Sources = append(bundle.Sources, p.Source)
		bundle.Conflicts = append(bundle.Conflicts, p.Conflicts...)
	}

	return bundle
}

func mergeFatigueWords(dst, src map[string]int) map[string]int {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]int, len(src))
	}
	for word, limit := range src {
		dst[word] = limit
	}
	return dst
}

func fatigueWordConflicts(sources []Parsed) []Conflict {
	type declaration struct {
		source string
		limit  int
	}
	byWord := make(map[string][]declaration)
	for _, p := range sources {
		for word, limit := range p.Structured.FatigueWords {
			if word == "" {
				continue
			}
			byWord[word] = append(byWord[word], declaration{source: p.Source, limit: limit})
		}
	}

	words := make([]string, 0, len(byWord))
	for word := range byWord {
		words = append(words, word)
	}
	sort.Strings(words)

	var conflicts []Conflict
	for _, word := range words {
		ds := byWord[word]
		if len(ds) < 2 {
			continue
		}
		first := ds[0].limit
		allSame := true
		for _, d := range ds[1:] {
			if d.limit != first {
				allSame = false
				break
			}
		}
		if allSame {
			continue
		}
		parts := make([]string, 0, len(ds))
		for _, d := range ds {
			parts = append(parts, fmt.Sprintf("%s=%d", d.source, d.limit))
		}
		winner := ds[len(ds)-1]
		conflicts = append(conflicts, Conflict{
			Source: winner.source,
			Kind:   ConflictFieldConflict,
			Field:  "fatigue_words." + word,
			Detail: fmt.Sprintf("Trường mệt_words[%q] được khai báo ở nhiều nguồn và các ngưỡng không nhất quán: %s; cái gần nhất có hiệu lực trước: %s",
				word, strings.Join(parts, " | "), winner.source),
		})
	}
	return conflicts
}

// allEqual xác định xem các giá trị của cùng một trường trong nhiều nguồn có hoàn toàn nhất quán hay không; nếu chúng nhất quán thì sẽ không có xung đột nào được báo cáo.
//
// Ngữ nghĩa của trường danh sách không quan tâm đến thứ tự, nhưng trong quá trình triển khai, quá trình khử lưu huỳnh yaml sẽ giữ nguyên thứ tự khai báo.
// Hai cấu hình giống hệt nhau phản ánh.DeepEqual trả về giá trị đúng, đáp ứng phán đoán "tính nhất quán về giá trị".
// Các trường hợp đặc biệt trong đó thứ tự khác nhau nhưng các thành phần giống nhau đều được chấp nhận và được coi là "không nhất quán" (vẫn chỉ là thông tin, không chặn).
func allEqual(field string, sources []Parsed) bool {
	if len(sources) < 2 {
		return true
	}
	first := extractField(field, sources[0].Structured)
	for _, p := range sources[1:] {
		if !reflect.DeepEqual(first, extractField(field, p.Structured)) {
			return false
		}
	}
	return true
}

func extractField(field string, s Structured) any {
	switch field {
	case "genre":
		return s.Genre
	case "chapter_words":
		if s.ChapterWords == nil {
			return nil
		}
		return *s.ChapterWords
	case "forbidden_chars":
		return s.ForbiddenChars
	case "forbidden_phrases":
		return s.ForbiddenPhrases
	case "fatigue_words":
		return s.FatigueWords
	default:
		return nil
	}
}

// mô tảFieldConflict Mô tả xung đột theo cách con người có thể đọc được: liệt kê tất cả các nguồn + giá trị cho mỗi nguồn.
// Đánh dấu nguồn hiệu ứng cuối cùng ở cuối (gần nhất đầu tiên).
func describeFieldConflict(field string, sources []Parsed) string {
	var parts []string
	for _, p := range sources {
		parts = append(parts, fmt.Sprintf("%s=%s", p.Source, formatFieldValue(field, p.Structured)))
	}
	winner := sources[len(sources)-1]
	return fmt.Sprintf(
		"Trường %s được khai báo ở nhiều nguồn và có các giá trị không nhất quán: %s; cái gần nhất có hiệu lực trước: %s",
		field, strings.Join(parts, " | "), winner.Source,
	)
}

func formatFieldValue(field string, s Structured) string {
	switch field {
	case "genre":
		return s.Genre
	case "chapter_words":
		if s.ChapterWords == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%d-%d", s.ChapterWords.Min, s.ChapterWords.Max)
	case "forbidden_chars":
		return fmt.Sprintf("%v", s.ForbiddenChars)
	case "forbidden_phrases":
		return fmt.Sprintf("%v", s.ForbiddenPhrases)
	case "fatigue_words":
		return fmt.Sprintf("%v", s.FatigueWords)
	default:
		return "<unknown>"
	}
}
