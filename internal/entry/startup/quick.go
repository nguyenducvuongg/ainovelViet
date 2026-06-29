package startup

import (
	"fmt"
	"strings"

	"github.com/nguyenducvuongg/ainovelViet/internal/host"
)

// Chuẩn bị nhanh tổ chức đầu vào trực tiếp thành một kế hoạch bắt đầu nhanh có thể được đưa vào Công cụ.
func PrepareQuick(req Request) (Plan, error) {
	prompt := strings.TrimSpace(req.UserPrompt)
	if prompt == "" {
		return Plan{}, fmt.Errorf("prompt is required")
	}
	return Plan{
		Mode:        ModeQuick,
		DisplayName: "bắt đầu nhanh",
		StartPrompt: host.BuildStartPrompt(prompt),
	}, nil
}
