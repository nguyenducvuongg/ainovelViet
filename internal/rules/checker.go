package rules

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Kiểm tra thực hiện kiểm tra cơ học trên văn bản chương theo các quy tắc có cấu trúc và trả về danh sách các sự kiện vi phạm.
//
// Hợp đồng thiết kế:
//   - Chỉ trả lại sự thật, không đưa ra hướng dẫn (Iron Rule 1)
//   - Không chặn bất kỳ quá trình người gọi nào
//   - ánh xạ cố định mức độ nghiêm trọng theo loại quy tắc (xem bảng chú thích type.go)
//
// tham số:
//   - text: nội dung chương (bản thảo cuối cùng hoặc bản nháp đều được chấp nhận)
//   - wordCount: Số lượng từ của chương (rune count). <0, người kiểm tra sẽ tự tính toán để tránh việc người gọi lặp lại việc quét O(n).
//   - s: các quy tắc có cấu trúc được hợp nhất; nil được trả về trực tiếp khi IsEmpty.
func Check(text string, wordCount int, s Structured) []Violation {
	if s.IsEmpty() {
		return nil
	}
	if wordCount < 0 {
		wordCount = utf8.RuneCountInString(text)
	}

	var violations []Violation
	violations = appendForbiddenChars(violations, text, s.ForbiddenChars)
	violations = appendForbiddenPhrases(violations, text, s.ForbiddenPhrases)
	violations = appendFatigueWords(violations, text, s.FatigueWords)
	violations = appendChapterWords(violations, wordCount, s.ChapterWords)
	return violations
}

// cấm_chars: xảy ra lỗi nếu nó xuất hiện ≥1 lần.
// Quy tắc tương tự chỉ tạo ra một vi phạm và số lần vi phạm thực tế là.
func appendForbiddenChars(vs []Violation, text string, list []string) []Violation {
	for _, ch := range list {
		if ch == "" {
			continue
		}
		n := strings.Count(text, ch)
		if n == 0 {
			continue
		}
		vs = append(vs, Violation{
			Rule:     "forbidden_chars",
			Target:   ch,
			Actual:   n,
			Severity: SeverityError,
		})
	}
	return vs
}

// bị cấm: Nếu nó xuất hiện ≥1 lần thì đó là lỗi; hành vi nhất quán với các ký tự bị cấm, chỉ có tên quy tắc mới phân biệt được nó.
func appendForbiddenPhrases(vs []Violation, text string, list []string) []Violation {
	for _, ph := range list {
		if ph == "" {
			continue
		}
		n := strings.Count(text, ph)
		if n == 0 {
			continue
		}
		vs = append(vs, Violation{
			Rule:     "forbidden_phrases",
			Target:   ph,
			Actual:   n,
			Severity: SeverityError,
		})
	}
	return vs
}

// Fatigue_words: Là vi phạm khi số lần xuất hiện trong chương này vượt quá ngưỡng, mức cảnh báo.
// Không tích lũy giữa các chương—các vấn đề xuyên chương sẽ được đưa ra để chẩn đoán sau.
func appendFatigueWords(vs []Violation, text string, m map[string]int) []Violation {
	for word, limit := range m {
		if word == "" || limit <= 0 {
			continue
		}
		n := strings.Count(text, word)
		if n <= limit {
			continue
		}
		vs = append(vs, Violation{
			Rule:     "fatigue_words",
			Target:   word,
			Limit:    limit,
			Actual:   n,
			Severity: SeverityWarning,
		})
	}
	return vs
}

// chap_words: độ lệch số từ.
// Độ lệch < 20%: cảnh báo; độ lệch ≥ 20%: sai số.
// Công thức độ lệch: dưới min, sử dụng (min-actual)/min; trên mức tối đa, sử dụng (actual-max)/max.
func appendChapterWords(vs []Violation, wordCount int, rng *WordRange) []Violation {
	if rng == nil {
		return vs
	}
	var deviation float64
	switch {
	case wordCount < rng.Min:
		if rng.Min == 0 {
			return vs
		}
		deviation = float64(rng.Min-wordCount) / float64(rng.Min)
	case wordCount > rng.Max:
		if rng.Max == 0 {
			return vs
		}
		deviation = float64(wordCount-rng.Max) / float64(rng.Max)
	default:
		return vs // trong phạm vi
	}

	severity := SeverityWarning
	if deviation >= ChapterWordsDeviationThreshold {
		severity = SeverityError
	}
	vs = append(vs, Violation{
		Rule:      "chapter_words",
		Limit:     fmt.Sprintf("%d-%d", rng.Min, rng.Max),
		Actual:    wordCount,
		Deviation: deviation,
		Severity:  severity,
	})
	return vs
}
