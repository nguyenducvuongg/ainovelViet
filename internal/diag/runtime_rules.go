package diag

import (
	"fmt"
	"strings"

	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// Ngưỡng phát hiện thời gian chạy.
const (
	repeatCritical = 8 // Khi số lần lặp lại gần kết thúc đạt đến con số này, nó sẽ trở nên quan trọng.
	streamIdleWarn = 3 // ngưỡng cảnh báo tích lũy streaming_idle
)

// RuntimeRuleFunc là chữ ký thống nhất của các quy tắc chẩn đoán thời gian chạy (tương ứng với RuleFunc ở phía tác giả).
// Tham số đầu vào là RuntimeCapture sau khi giải mẫn cảm và tổng hợp, đồng thời loại báo cáo đầu ra Tìm kiếm - tất cả đều Tự động,
// Chỉ chẩn đoán, không tạo ra hành động (kỷ luật người quan sát, xem architecture.md §2.3).
type RuntimeRuleFunc func(rc *RuntimeCapture) []Finding

var runtimeRules = []RuntimeRuleFunc{
	repeatedErrors,
	stuckStep,
	streamIdleStorm,
}

// RuntimeFindings chạy tất cả các quy tắc thời gian chạy.
func runtimeFindings(rc *RuntimeCapture) []Finding {
	var out []Finding
	for _, rule := range runtimeRules {
		out = append(out, rule(rc)...)
	}
	return out
}

// Chẩn đoán là mục nhập chẩn đoán hoàn chỉnh cho /diag: chẩn đoán tạo + tín hiệu thời gian chạy + phát hiện thời gian chạy,
// Trả lại Báo cáo đã hợp nhất và RuntimeCapture ban đầu (để tái sử dụng xuất nhằm tránh thu thập thông tin lặp lại).
// Kết quả trong thời gian chạy chỉ được hợp nhất thành Kết quả để trình bày và Hành động không được thay đổi - giữ nguyên sự quan sát thuần túy.
func Diagnose(s *store.Store) (Report, RuntimeCapture) {
	rep := Analyze(s)
	rc := CaptureRuntime(s)
	rep.Findings = append(rep.Findings, runtimeFindings(&rc)...)
	sortFindings(rep.Findings)
	return rep, rc
}

// lặp lạiErrors chỉ xác định "lỗi gần cuối định kỳ/tham số không hợp lệ" là Đang tìm.
// Không chạm vào các công cụ thông thường và lặp lại chúng - subagent/novel_context/read_chapter, v.v. là điều đương nhiên khi chạy đường dài.
// Tần số cao, số lần tích lũy không phải là tín hiệu tuần hoàn; sự "lặp lại mà không tiến bộ" thực sự được ghi lại bởi StedStep.
func repeatedErrors(rc *RuntimeCapture) []Finding {
	var out []Finding
	for _, r := range rc.Repeats {
		var rule, title, sugg string
		switch {
		case strings.Contains(r.Sig, " · err: "):
			rule = "RepeatedToolError"
			title = "Công cụ báo lỗi tương tự nhiều lần"
			sugg = "Cùng một công cụ gần cuối liên tục trả về cùng một lỗi, chủ yếu là do các tham số mô hình không nhất quán hoặc hợp đồng công cụ không nhất quán; kiểm tra quy ước tham số nhắc/xác minh công cụ Agentcore (xem #34)."
		case strings.Contains(r.Sig, "(args invalid)"):
			rule = "ArgsInvalidLoop"
			title = "Không thể phân tích cú pháp tham số nhiều lần"
			sugg = "Các tham số do mô hình gửi không thể được phân tích cú pháp nhưng được thử lại liên tục; xem liệu Agentcore có phân loại lỏng lẻo cho loại này hay không (xem #34)."
		default:
			continue // Các công cụ thông thường sẽ vô dụng nếu lặp đi lặp lại. Tìm kiếm
		}
		sev := SevWarning
		if r.Count >= repeatCritical {
			sev = SevCritical
		}
		out = append(out, Finding{
			Rule:       rule,
			Category:   CatFlow,
			Severity:   sev,
			Confidence: ConfHigh,
			AutoLevel:  AutoNone,
			Target:     "runtime.flow",
			Title:      title,
			Evidence:   fmt.Sprintf("`%s` ×%d", r.Sig, r.Count),
			Suggestion: sugg,
		})
	}
	return out
}

// bị kẹtStep phát hiện các điểm kiểm tra liên tục dừng ở cùng một bước.
func stuckStep(rc *RuntimeCapture) []Finding {
	if rc.StuckStep == "" {
		return nil
	}
	sev := SevWarning
	if rc.StuckCount >= repeatCritical {
		sev = SevCritical
	}
	return []Finding{{
		Rule:       "StuckStep",
		Category:   CatFlow,
		Severity:   sev,
		Confidence: ConfHigh,
		AutoLevel:  AutoNone,
		Target:     "runtime.flow",
		Title:      "trạm kiểm soát bị kẹt ở cùng một bước",
		Evidence:   fmt.Sprintf("Dừng liên tục ở `%s`×%d", rc.StuckStep, rc.StuckCount),
		Suggestion: "Cùng một bước được viết đi viết lại mà không tiến lên; kết hợp các chữ ký lặp lại ở trên để xác định tác nhân phụ nào bị kẹt.",
	}}
}

// StreamIdleStorm phát hiện tình trạng gián đoạn phát trực tuyến thường xuyên (#32).
func streamIdleStorm(rc *RuntimeCapture) []Finding {
	n := rc.LogKinds["stream_idle"]
	if n < streamIdleWarn {
		return nil
	}
	return []Finding{{
		Rule:       "StreamIdleStorm",
		Category:   CatFlow,
		Severity:   SevWarning,
		Confidence: ConfHigh,
		AutoLevel:  AutoNone,
		Target:     "runtime.provider",
		Title:      "Sự gián đoạn phát trực tuyến thường xuyên (stream_idle)",
		Evidence:   fmt.Sprintf("stream_idle ×%d", n),
		Suggestion: "Thượng nguồn hồi lâu không phun ra thẻ bài, ngoài ý muốn bị giám sát giết chết; mô hình suy nghĩ chậm làm tăng streamingIdleTimeout hoặc kiểm tra độ ổn định kết nối của nhà cung cấp (xem #32).",
	}}
}
