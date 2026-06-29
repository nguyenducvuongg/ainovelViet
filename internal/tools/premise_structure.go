package tools

import (
	"strings"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
)

var premiseHeadingAliases = map[string]string{
	"định vị chủ đề": "Định vị chủ đề",
	"chủ đề và giọng điệu": "chủ đề và giọng điệu",
	"chủ đề và giai điệu": "chủ đề và giọng điệu",
	"xung đột cốt lõi": "xung đột cốt lõi",
	"mục tiêu của nhân vật chính": "mục tiêu của nhân vật chính",
	"hướng kết thúc": "hướng cuối cùng",
	"hướng đi cuối cùng": "hướng cuối cùng",
	"hướng cuối cùng": "hướng cuối cùng",
	"khu vực cấm viết": "Khu vực cấm viết",
	"vùng hạn chế viết": "Khu vực cấm viết",
	"vùng cấm viết": "Khu vực cấm viết",
	"điểm bán hàng khác biệt": "Điểm bán hàng khác biệt",
	"móc phân biệt": "Móc phân biệt",
	"cốt lõi thực hiện lời hứa": "Cốt lõi thực hiện lời hứa",
	"thực hiện cốt lõi những lời hứa": "Cốt lõi thực hiện lời hứa",
	"công cụ câu chuyện": "công cụ câu chuyện",
	"mối quan hệ/chủ đề tăng trưởng": "Mối quan hệ/Chủ đề tăng trưởng",
	"đường dẫn nâng cấp": "Đường dẫn nâng cấp",
	"giữa lượt": "giữa lượt",
	"bước ngoặt giữa kỳ": "giữa lượt",
	"bước ngoặt đoạn giữa": "giữa lượt",
	"lượt chuyển tiếp giữa kỳ": "giữa lượt",
	"đề xuất cuối cùng": "đề xuất cuối cùng",
	"khả năng thích ứng truyện ngắn": "khả năng thích ứng truyện ngắn",
	"tại sao cuốn sách này phù hợp với truyện ngắn/tập đơn?": "khả năng thích ứng truyện ngắn",
	"tại sao cuốn sách này lại phù hợp với truyện ngắn/tập đơn?": "khả năng thích ứng truyện ngắn",
}

func parsePremiseSections(premise string) map[string]string {
	lines := strings.Split(premise, "\n")
	sections := make(map[string]string)
	var current string
	var body []string

	flush := func() {
		if current == "" {
			return
		}
		text := strings.TrimSpace(strings.Join(body, "\n"))
		if text != "" {
			sections[current] = text
		}
		body = body[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if heading, ok := canonicalPremiseHeading(trimmed); ok {
			flush()
			current = heading
			continue
		}
		if current != "" {
			body = append(body, line)
		}
	}
	flush()
	return sections
}

func canonicalPremiseHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	title := strings.TrimSpace(strings.TrimLeft(line, "#"))
	titleLower := strings.ToLower(title)
	canonical, ok := premiseHeadingAliases[titleLower]
	return canonical, ok
}

func premiseStructure(premise string, tier domain.PlanningTier) map[string]any {
	sections := parsePremiseSections(premise)
	required := requiredPremiseHeadings(tier)
	found := make([]string, 0, len(required))
	var missing []string
	for _, heading := range required {
		if _, ok := sections[heading]; ok {
			found = append(found, heading)
			continue
		}
		missing = append(missing, heading)
	}

	structure := map[string]any{
		"template_ready": len(missing) == 0,
		"found":          found,
		"missing":        missing,
	}
	if len(sections) > 0 {
		structure["section_count"] = len(sections)
	}
	return structure
}

func requiredPremiseHeadings(tier domain.PlanningTier) []string {
	common := []string{
		"chủ đề và giọng điệu",
		"Định vị chủ đề",
		"xung đột cốt lõi",
		"mục tiêu của nhân vật chính",
		"hướng cuối cùng",
		"Khu vực cấm viết",
		"Điểm bán hàng khác biệt",
		"Móc phân biệt",
		"Cốt lõi thực hiện lời hứa",
	}

	switch tier {
	case domain.PlanningTierLong:
		return append(common,
			"công cụ câu chuyện",
			"Mối quan hệ/Chủ đề tăng trưởng",
			"Đường dẫn nâng cấp",
			"giữa lượt",
			"đề xuất cuối cùng",
		)
	case domain.PlanningTierMid:
		return append(common,
			"công cụ câu chuyện",
			"giữa lượt",
		)
	case domain.PlanningTierShort:
		return append(common,
			"khả năng thích ứng truyện ngắn",
		)
	default:
		return common
	}
}
