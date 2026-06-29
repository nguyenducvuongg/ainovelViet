package diag

import "fmt"

// PlanActions tạo ra các hành động thực thi dựa trên các Kết quả có độ tin cậy cao.
// Chỉ Tìm kiếm với Sự tự tin==cao && AutoLevel==safe mới tạo ra Hành động.
func PlanActions(findings []Finding) []Action {
	var actions []Action
	seen := make(map[string]struct{})

	for _, f := range findings {
		if f.Confidence != ConfHigh || f.AutoLevel != AutoSafe {
			continue
		}
		if _, ok := seen[f.Rule]; ok {
			continue
		}
		seen[f.Rule] = struct{}{}

		actions = append(actions, planRule(f)...)
	}
	return actions
}

func planRule(f Finding) []Action {
	key := findingFingerprint(f)

	switch f.Rule {
	case "PhaseFlowMismatch":
		return []Action{
			{SourceRule: f.Rule, Kind: ActionEmitNotice, Severity: f.Severity, Summary: f.Title, Message: f.Title, Fingerprint: key},
			{SourceRule: f.Rule, Kind: ActionEnqueueFollowUp, Severity: f.Severity, Summary: "Sửa chữa ngoại lệ máy trạng thái", Message: "Ngoại lệ máy trạng thái:" + f.Evidence + ". Vui lòng kiểm tra và sửa trạng thái pha/luồng của tiến trình trước khi tiếp tục.", Fingerprint: key},
		}
	case "OutlineExhausted":
		return []Action{
			{SourceRule: f.Rule, Kind: ActionEnqueueFollowUp, Severity: f.Severity, Summary: "Phác thảo xử lý cạn kiệt", Message: "Số lượng chương hoàn thành đã đạt đến giới hạn trên theo kế hoạch. Vui lòng gọi cho Architect trước để mở rộng phần tiếp theo hoặc thêm tập mới trước khi tiếp tục viết.", Fingerprint: key},
		}
	case "OrphanedSteer":
		return []Action{
			{SourceRule: f.Rule, Kind: ActionEnqueueFollowUp, Severity: f.Severity, Summary: "Tiêu thụ sự can thiệp của người dùng chưa được xử lý", Message: "Có hướng dẫn can thiệp của người dùng chưa sử dụng. Vui lòng ưu tiên chỉ đạo đang chờ xử lý trước khi tiếp tục nhiệm vụ hiện tại.", Fingerprint: key},
		}
	default:
		return nil
	}
}

func findingFingerprint(f Finding) string {
	return fmt.Sprintf("%s|%s|%s|%s", f.Rule, f.Target, f.Title, f.Evidence)
}
