package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/voocel/ainovel-cli/internal/host"
)

// renderInputBox hiển thị vùng nhập liệu phía dưới.
// Hộp nhập liệu hoàn toàn chịu trách nhiệm về việc nhập liệu và lời nhắc và không lưu trữ thanh chế độ khởi động.
func renderInputBox(inputView, hints string, snap host.UISnapshot, outputDir string, width int) string {
	innerW := width - 4 // border + padding
	if innerW < 12 {
		innerW = 12
	}

	// Dòng đầu vào: dấu nhắc + hộp đầu vào
	prompt := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("❯ ")
	inputLine := prompt + inputView

	// Dòng nhắc: phím tắt bên trái, tiến trình bên phải
	info := buildRightInfo(snap, outputDir)
	line2 := joinInlineSides(hints, info, innerW)

	// Khu vực nhập liệu (hộp đơn để tránh các hộp nhập liệu kép trực quan)
	inputStyle := lipgloss.NewStyle().
		Width(width).
		Border(baseBorder, true, false, true, false).
		BorderForeground(colorDim).
		Padding(0, 1)
	inputBlock := inputStyle.Render(inputLine)

	// Dòng nhắc (không có viền, ngay dưới dấu gạch dưới)
	hintStyle := lipgloss.NewStyle().
		Width(width).
		Padding(0, 2)
	hintBlock := hintStyle.Render(line2)

	return inputBlock + "\n" + hintBlock + "\n"
}

// buildRightInfo Xây dựng thông tin phù hợp: nhà cung cấp · mô hình (cửa sổ) · chi phí · thư mục.
// Thông tin về tiến trình như số chương/số từ được hiển thị trên bảng "Tổng quan" ở bên trái và sẽ không được lặp lại ở đây.
func buildRightInfo(snap host.UISnapshot, outputDir string) string {
	var parts []string

	if snap.Provider != "" {
		parts = append(parts, snap.Provider)
	}
	if snap.ModelName != "" {
		if w := formatContextWindow(snap.ModelContextWindow); w != "" {
			parts = append(parts, snap.ModelName+"("+w+")")
		} else {
			parts = append(parts, snap.ModelName)
		}
	}
	if cost := formatCostUSD(snap.TotalCostUSD); cost != "" {
		parts = append(parts, cost)
	}
	if outputDir != "" {
		parts = append(parts, "./"+filepath.Base(outputDir))
	}

	if len(parts) == 0 {
		return lipgloss.NewStyle().Foreground(colorDim).Render("READY")
	}
	return lipgloss.NewStyle().Foreground(colorDim).Render(strings.Join(parts, " · "))
}

func joinInlineSides(left, right string, width int) string {
	if width <= 0 {
		return left + right
	}
	if strings.TrimSpace(right) == "" {
		return fitInlineLine(left, width)
	}

	right = fitInlineLine(right, width)
	rightW := ansi.StringWidth(right)
	if rightW >= width {
		return right
	}

	leftMax := width - rightW - 1
	if leftMax < 0 {
		leftMax = 0
	}
	left = fitInlineLine(left, leftMax)
	gap := width - ansi.StringWidth(left) - rightW
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func fitInlineLine(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) <= width {
		return text
	}
	return ansi.Truncate(text, width, "...")
}
