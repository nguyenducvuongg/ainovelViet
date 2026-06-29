package tui

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/nguyenducvuongg/ainovelViet/internal/host"
	"github.com/nguyenducvuongg/ainovelViet/internal/utils"
)

// renderTopBar hiển thị thanh trạng thái trên cùng.
// Bên trái: nhà cung cấp/người mẫu, giữa: tên sách, bên phải: bảng trạng thái.
func renderTopBar(snap host.UISnapshot, width int, spinnerFrame, version string) string {
	novelName := snap.NovelName
	if novelName == "" {
		novelName = "Tên sách chưa quyết định"
	}

	var infoParts []string
	if version != "" {
		infoParts = append(infoParts, "ainovel-cli "+version)
	}
	if snap.Provider != "" {
		infoParts = append(infoParts, snap.Provider)
	}
	if snap.ModelName != "" {
		if w := formatContextWindow(snap.ModelContextWindow); w != "" {
			infoParts = append(infoParts, snap.ModelName+"("+w+")")
		} else {
			infoParts = append(infoParts, snap.ModelName)
		}
	}
	if snap.Style != "" && snap.Style != "default" {
		infoParts = append(infoParts, snap.Style)
	}
	leftText := strings.Join(infoParts, " · ")

	label := snap.StatusLabel
	if label == "" {
		label = "READY"
	}
	color, ok := statusColors[label]
	if !ok {
		color = colorDim
	}
	disp, ok := statusDisplay[label]
	if !ok {
		disp = struct {
			icon  string
			label string
		}{"○", strings.ToLower(label)}
	}
	icon := disp.icon
	if snap.IsRunning && spinnerFrame != "" {
		icon = spinnerFrame
	}
	var status string
	if icon != "" {
		status = statusIconStyle.Foreground(color).Render(icon) + " " + statusLabelStyle.Render(disp.label)
	} else {
		status = statusLabelStyle.Render(disp.label)
	}

	innerW := max(12, width-2)
	titleText := truncate(novelName, max(8, innerW/3))
	centerW := max(16, lipgloss.Width(titleText)+6)
	if centerW > innerW-24 {
		centerW = max(8, innerW-24)
	}
	sideTotal := innerW - centerW
	if sideTotal < 0 {
		sideTotal = 0
		centerW = innerW
	}
	leftW := sideTotal / 2
	rightW := innerW - centerW - leftW

	leftCell := lipgloss.NewStyle().
		Width(leftW).
		AlignHorizontal(lipgloss.Left).
		Foreground(colorDim).
		Render(truncate(leftText, leftW))
	centerCell := lipgloss.NewStyle().
		Width(centerW).
		AlignHorizontal(lipgloss.Center).
		Bold(true).
		Foreground(bodyTextColor).
		Render(titleText)
	rightCell := lipgloss.NewStyle().
		Width(rightW).
		AlignHorizontal(lipgloss.Right).
		Render(status)

	content := leftCell + centerCell + rightCell
	return topBarStyle.Width(width).
		Border(baseBorder, false, false, true, false).
		BorderForeground(colorDim).
		Render(content)
}

// renderStatePanel gói nội dung thanh bên trạng thái (đã ở trạng thái VP) vào một hộp có đường viền bên phải ở bên trái.
// Đối xứng với renderDetailPanel: nội dung được tạo bởi renderStateContent và đưa vào khung nhìn, khung nhìn này chỉ chịu trách nhiệm về khung.
// MaxHeight kẹp chiều cao để ngăn cửa sổ tràn cao hơn cột bên phải khi cửa sổ bị giảm (xem hợp đồng chiều cao của panel_test.go).
func renderStatePanel(vp viewport.Model, width, height int, focused bool) string {
	borderColor := colorDim
	if focused {
		borderColor = colorAccent
	}
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxHeight(height).
		Border(baseBorder, false, true, false, false).
		BorderForeground(borderColor).
		Padding(1, 1)
	return style.Render(vp.View())
}

// renderStateContent tạo nội dung thuần túy của thanh bên trạng thái (không có viền/khung bên ngoài) để stateVP.SetContent sử dụng.
func renderStateContent(snap host.UISnapshot, contentW int) string {
	contentW = max(12, contentW)
	agents := sidebarAgents(snap.Agents)
	idleAgents := sidebarIdleAgents(snap.Agents)
	var sections []string

	if snap.RecoveryLabel != "" {
		sections = append(sections, lipgloss.NewStyle().Foreground(colorMuted).Italic(true).
			Render(truncate(snap.RecoveryLabel, contentW)))
	}

	var overview strings.Builder
	overview.WriteString(renderField("Trạng thái chạy", snapshotRuntimeStateLabel(snap.RuntimeState)))
	overview.WriteString(renderField("sân khấu", snapshotPhaseLabel(snap.Phase)))
	overview.WriteString(renderField("quá trình", snapshotFlowLabel(snap.Flow)))
	if snap.Layered {
		overview.WriteString(renderField("Hoàn thành", fmt.Sprintf("Chương %d", snap.CompletedCount)))
		// Lập trình động phân cấp: Cột bên phải chỉ hiển thị các chương mở rộng của cung hiện tại và "đã lên kế hoạch" cũng sử dụng cùng cỡ nòng.
		// Nếu không, các ước tính sơ bộ về các Chương ước tính của cung xương (chẳng hạn như 92) sẽ được trộn lẫn vào, sẽ không khớp với đường viền hiển thị.
		// Progress.TotalChapters Giá trị đó chỉ được sử dụng cho các quyết định ContextProfile nội bộ và không được rò rỉ ra giao diện người dùng.
		if planned := len(snap.Outline); planned > 0 {
			overview.WriteString(renderField("Đã lên kế hoạch", fmt.Sprintf("Chương %d", planned)))
		}
	} else {
		switch {
		case snap.TotalChapters > 0:
			overview.WriteString(renderField("lịch trình", fmt.Sprintf("Chương %d/%d", snap.CompletedCount, snap.TotalChapters)))
		default:
			overview.WriteString(renderField("Hoàn thành", fmt.Sprintf("Chương %d", snap.CompletedCount)))
		}
	}
	overview.WriteString(renderField("số từ", formatNumber(snap.TotalWordCount)))
	if label, ch := inProgressDisplay(snap); label != "" {
		overview.WriteString(renderField(label, fmt.Sprintf("Chương %d", ch)))
	}
	if headline := snapshotHeadline(snap); headline != "" {
		label := "hiện hành"
		if !snap.IsRunning {
			label = "Để được khôi phục"
		}
		overview.WriteString(renderHighlightField(label, truncate(headline, contentW-10)))
	}
	sections = append(sections, renderSidebarSection("Tổng quan", overview.String(), contentW))

	if len(agents) > 0 {
		var agentBody strings.Builder
		for _, agent := range agents {
			agentBody.WriteString(renderAgentLine(agent, contentW))
			agentBody.WriteString("\n")
		}
		if len(idleAgents) > 0 {
			agentBody.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("Đang có cuộc gọi: " + truncate(strings.Join(idleAgents, " · "), max(8, contentW-2))))
			agentBody.WriteString("\n")
		}
		sections = append(sections, renderSidebarSection("Chạy vai trò", agentBody.String(), contentW))
	}

	if len(snap.PendingRewrites) > 0 {
		var rewrite strings.Builder
		rewrite.WriteString(renderHighlightField("xếp hàng", fmt.Sprintf("%v", snap.PendingRewrites)))
		if snap.RewriteReason != "" {
			rewrite.WriteString(renderField("lý do", truncate(snap.RewriteReason, contentW-10)))
		}
		sections = append(sections, renderSidebarSection("Làm lại", rewrite.String(), contentW))
	}

	if snap.PendingSteer != "" {
		sections = append(sections, renderSidebarSection("sự can thiệp",
			renderHighlightField("Chưa giải quyết", truncate(snap.PendingSteer, contentW-10)), contentW))
	}

	if body := renderUsageSidebar(snap, contentW); body != "" {
		sections = append(sections, renderSidebarSection("liều lượng", body, contentW))
	}

	if body := renderCacheSidebar(snap, contentW); body != "" {
		sections = append(sections, renderSidebarSection("bộ nhớ đệm", body, contentW))
	}

	if body := renderContextSidebar(snap, contentW); body != "" {
		sections = append(sections, renderSidebarSection("bối cảnh", body, contentW))
	}

	return strings.Join(sections, "\n\n")
}

func renderAgentLine(agent host.AgentSnapshot, width int) string {
	stateColor := taskStatusColor(agent.State)
	icon := lipgloss.NewStyle().Foreground(stateColor).Render(agentStateIcon(agent.State))
	badge := lipgloss.NewStyle().Foreground(stateColor).Render(agentStateLabel(agent.State))
	name := lipgloss.NewStyle().Bold(true).Foreground(bodyTextColor).Render(agentDisplayName(agent.Name))
	line := icon + " " + name + " " + badge

	taskLine := agentTaskLine(agent)
	if taskLine != "" {
		line += "\n" + lipgloss.NewStyle().Foreground(colorDim).Render("  "+truncate(taskLine, max(8, width-2)))
	}

	detail := agent.Summary
	if agent.Tool != "" {
		detail = agent.Tool
	}
	if agent.State == "idle" && detail == "Chế độ chờ" {
		detail = ""
	}
	if detail != "" && detail != taskLine {
		line += "\n" + lipgloss.NewStyle().Foreground(colorMuted).Render("  "+truncate(detail, max(8, width-2)))
	}
	if ctx := agentContextLine(agent); ctx != "" {
		line += "\n" + lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("  "+truncate(ctx, max(8, width-2)))
	}
	return line
}

func renderSidebarSection(title, body string, width int) string {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return ""
	}
	lineW := max(0, width-lipgloss.Width(title)-1)
	header := panelTitleStyle.Render(title) + " " +
		lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("─", lineW))
	card := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorDim).
		PaddingLeft(1).
		Render(body)
	return header + "\n" + card
}

func sidebarAgents(agents []host.AgentSnapshot) []host.AgentSnapshot {
	var out []host.AgentSnapshot
	for _, agent := range agents {
		if agent.State == "idle" {
			continue
		}
		out = append(out, agent)
	}
	if len(out) == 0 {
		out = append(out, agents...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := out[i], out[j]
		if agentStateRank(li.State) != agentStateRank(lj.State) {
			return agentStateRank(li.State) < agentStateRank(lj.State)
		}
		return agentOrder(li.Name) < agentOrder(lj.Name)
	})
	return out
}

func sidebarIdleAgents(agents []host.AgentSnapshot) []string {
	var names []string
	hasActive := false
	for _, agent := range agents {
		if agent.State != "idle" {
			hasActive = true
			continue
		}
		names = append(names, agentDisplayName(agent.Name))
	}
	if !hasActive {
		return nil
	}
	sort.Strings(names)
	return names
}

// inProgressDisplay tính toán nhãn và số chương của trường "Đang tiến hành".
// Chọn động từ (đánh bóng/viết lại/viết) dựa trên dòng chảy; khi in_progress_chapter không khớp với luồng, nó được coi là cũ:
//   - Trong chế độ đánh bóng/viết lại, chương không ở trạng thái chờ_rewrites → quay lại chương đầu tiên trong hàng đợi
//   - Không hiển thị khi trường bằng 0
func inProgressDisplay(snap host.UISnapshot) (label string, chapter int) {
	ch := snap.InProgressChapter
	switch snap.Flow {
	case "polishing":
		if ch <= 0 || !slices.Contains(snap.PendingRewrites, ch) {
			if len(snap.PendingRewrites) == 0 {
				return "", 0
			}
			ch = snap.PendingRewrites[0]
		}
		return "đánh bóng", ch
	case "rewriting":
		if ch <= 0 || !slices.Contains(snap.PendingRewrites, ch) {
			if len(snap.PendingRewrites) == 0 {
				return "", 0
			}
			ch = snap.PendingRewrites[0]
		}
		return "Viết lại", ch
	default:
		if ch <= 0 {
			return "", 0
		}
		return "Viết", ch
	}
}

func snapshotHeadline(snap host.UISnapshot) string {
	if snap.PendingSteer != "" {
		if !snap.IsRunning {
			return "Đang chờ khôi phục: Xử lý sự can thiệp của người dùng"
		}
		return "Đang chờ xử lý sự can thiệp của người dùng"
	}
	if len(snap.PendingRewrites) > 0 {
		if !snap.IsRunning {
			return "Đang chờ phục hồi: làm lại"
		}
		return "Đang chờ làm lại"
	}
	return ""
}

func snapshotPhaseLabel(phase string) string {
	switch phase {
	case "premise":
		return "tiền đề"
	case "outline":
		return "phác thảo"
	case "writing":
		return "viết"
	case "complete":
		return "Hoàn thành"
	case "init":
		return "khởi tạo"
	default:
		if phase == "" {
			return "-"
		}
		return phase
	}
}

func snapshotRuntimeStateLabel(state string) string {
	switch state {
	case "running":
		return "Đang chạy"
	case "pausing":
		return "Đã tạm dừng"
	case "paused":
		return "Cấm"
	case "completed":
		return "Hoàn thành"
	default:
		return "nhàn rỗi"
	}
}

func snapshotFlowLabel(flow string) string {
	switch flow {
	case "":
		return "-"
	case "writing":
		return "viết"
	case "reviewing":
		return "Ôn tập"
	case "rewriting":
		return "viết lại"
	case "polishing":
		return "đánh bóng"
	case "steering":
		return "sự can thiệp"
	default:
		return flow
	}
}

func renderUsageSidebar(snap host.UISnapshot, width int) string {
	if snap.TotalInputTokens <= 0 && snap.TotalOutputTokens <= 0 && snap.TotalCostUSD <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(renderField("đi vào", formatTokensCompact(snap.TotalInputTokens)))
	b.WriteString(renderField("đầu ra", formatTokensCompact(snap.TotalOutputTokens)))
	if cost := formatCostUSD(snap.TotalCostUSD); cost != "" {
		b.WriteString(renderField("trị giá", cost))
	}
	if saved := formatCostUSD(snap.TotalSavedUSD); saved != "" {
		b.WriteString(renderField("cứu", saved))
	}
	if snap.BudgetLimitUSD > 0 {
		pct := snap.TotalCostUSD / snap.BudgetLimitUSD * 100
		b.WriteString(renderField("Ngân sách", fmt.Sprintf("$%.2f/$%.2f (%.0f%%)", snap.TotalCostUSD, snap.BudgetLimitUSD, pct)))
	}

	agentStats := usageStatsByCost(snap.CachePerAgent)
	if len(agentStats) > 0 {
		b.WriteString(renderUsageGroupHeader("Vai trò", width))
		limit := min(len(agentStats), 4)
		for i := 0; i < limit; i++ {
			a := agentStats[i]
			b.WriteString(renderUsageLine(agentDisplayName(a.Role), eventAgentColor(a.Role), a.Input, a.Output, a.Cost, width))
			b.WriteString("\n")
		}
	}
	modelStats := usageStatsByCost(snap.CachePerModel)
	if len(modelStats) > 0 {
		b.WriteString(renderUsageGroupHeader("Người mẫu", width))
		limit := min(len(modelStats), 4)
		for i := 0; i < limit; i++ {
			a := modelStats[i]
			b.WriteString(renderUsageLine(modelDisplayName(a.Model), bodyTextColor, a.Input, a.Output, a.Cost, width))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func usageStatsByCost(in []host.AgentCacheStat) []host.AgentCacheStat {
	out := append([]host.AgentCacheStat(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Input+out[i].Output > out[j].Input+out[j].Output
	})
	return out
}

func renderUsageGroupHeader(label string, width int) string {
	line := lipgloss.NewStyle().Foreground(colorDim).
		Render(strings.Repeat("·", max(8, width-lipgloss.Width(label)-3)))
	return lipgloss.NewStyle().Foreground(colorMuted).Render(label+" ") + line + "\n"
}

func renderUsageLine(name string, color lipgloss.TerminalColor, input, output int, cost float64, width int) string {
	nameW := 11
	if width < 24 {
		nameW = 8
	}
	nameCell := lipgloss.NewStyle().Foreground(color).Width(nameW).
		Render(truncate(name, nameW))
	tokens := formatTokensCompact(input + output)
	right := tokens
	if costStr := formatCostUSD(cost); costStr != "" {
		right += " · " + costStr
	}
	return fitInlineLine(nameCell+lipgloss.NewStyle().Foreground(colorDim).Render(right), width)
}

func modelDisplayName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "unknown"
	}
	parts := strings.Split(model, "/")
	if len(parts) >= 3 {
		return strings.Join(parts[1:], "/")
	}
	if len(parts) == 2 {
		return parts[1]
	}
	return model
}

// renderCacheSidebar hiển thị khối "cache" cột bên trái.
//
// Ba trạng thái:
//  1. Mã thông báo hoàn toàn không được sử dụng: trống được trả về và phần không được hiển thị.
//  2. Tất cả các vai trò trong phiên hiện tại đều là các mô hình đang chạy không hỗ trợ bộ nhớ đệm lời nhắc: chỉ một dòng lời nhắc "không bật" được hiển thị.
//  3. Đã bật: "Tích lũy tỷ lệ truy cập/Gần 10 · Đang lưu · Đọc/Ghi" hàng đầu + dấu phân cách + hàng cho mỗi vai trò
//
// Khi dòng mỗi vai trò có khả năng, nó sẽ hiển thị hai chữ số "tích lũy/gần 10%"; khi không có khả năng, nó sẽ hiển thị "không được bật".
// Có thể xác định "kéo sớm" và "lượt truy cập thấp ở trạng thái ổn định" bằng cách so sánh tích lũy với gần N lần.
func renderCacheSidebar(snap host.UISnapshot, width int) string {
	// Luồng ngược dòng không gửi đoạn sử dụng cuối cùng của OpenAI—dữ liệu tích lũy đều bằng 0.
	// Nhưng đây không phải là "bộ nhớ đệm không được kích hoạt" cũng không phải "mức sử dụng quá thấp và bị cổng ẩn". Nó phải được nhắc nhở một cách rõ ràng.
	// Nếu không, người dùng sẽ luôn tưởng rằng mã cache được ghi ở cột bên trái nhưng sẽ không hiển thị. Ưu tiên cao nhất.
	if snap.MissingAssistantUsage > 0 && snap.TotalInputTokens <= 0 {
		warn := lipgloss.NewStyle().Foreground(colorError).Bold(true).
			Render(fmt.Sprintf("⚠ Ngược dòng không trả lại mức sử dụng (%d lần)", snap.MissingAssistantUsage))
		hint := lipgloss.NewStyle().Foreground(colorDim).Italic(true).
			Render(truncate("Kiểm tra nhà cung cấp streaming_options.include_usage", max(8, width-2)))
		return warn + "\n" + hint + "\n"
	}

	if snap.TotalInputTokens <= 0 && snap.TotalCacheWriteTokens <= 0 {
		return ""
	}

	// Chưa bật gì cả → Hiển thị dòng giải thích để tránh bị người dùng đánh giá sai là "cần kiểm tra 0% lượt truy cập"
	if !snap.OverallCacheCapable && snap.TotalCacheReadTokens == 0 && snap.TotalCacheWriteTokens == 0 {
		return lipgloss.NewStyle().Foreground(colorDim).Italic(true).
			Render(truncate("bộ nhớ đệm nhắc nhở chưa được bật cho kiểu máy hiện tại", max(8, width-2))) + "\n"
	}

	var b strings.Builder

	// Các chỉ số tổng hợp ở trên cùng: tích lũy + gần N mỗi dòng chiếm một dòng, có nhãn rõ ràng, tránh "X% · gần N Y%"
	// Sự nhầm lẫn của ba dấu phân cách (dấu phần trăm/điểm giữa/chữ) dẫn đến ngữ nghĩa không rõ ràng.
	overallHit := cacheHitRate(snap.TotalCacheReadTokens, snap.TotalInputTokens)
	b.WriteString(renderField("Số lượt truy cập tích lũy", colorPercent(overallHit)))
	if snap.OverallRecentSamples > 0 && snap.OverallRecentInput > 0 {
		recent := cacheHitRate(snap.OverallRecentCacheRead, snap.OverallRecentInput)
		b.WriteString(renderField(fmt.Sprintf("Gần %d đạt", snap.OverallRecentSamples), colorPercent(recent)))
	}

	if savedStr := formatCostUSD(snap.TotalSavedUSD); savedStr != "" {
		b.WriteString(renderField("cứu", savedStr))
	}

	// Khối lượng đọc/ghi được chia thành hai dòng. Khối lượng ghi bằng 0 là tiêu chuẩn trong các giao thức chuỗi OpenAI/Gemini——
	// Hai công ty này sử dụng bộ nhớ đệm minh bạch tự động và việc ghi bộ nhớ đệm hoàn toàn miễn phí (lần bỏ lỡ đầu tiên là ở mức giá đầu vào thông thường.
	// Không có phí bảo hiểm cho việc tạo bộ đệm), do đó bản thân giao thức không hiển thị trường cache_creation, điều này là không cần thiết.
	// Chỉ có bộ Anthropic/Bedrock mới báo cáo số lượng bài viết vì họ tính phí viết (5 triệu +25%/1 giờ +100%).
	// Số tiền này phải được cung cấp cho người dùng để thanh toán.
	b.WriteString(renderField("Đọc bộ nhớ đệm", formatTokensCompact(snap.TotalCacheReadTokens)))
	if snap.TotalCacheWriteTokens > 0 {
		b.WriteString(renderField("Khối lượng ghi bộ nhớ đệm", formatTokensCompact(snap.TotalCacheWriteTokens)))
	} else if snap.TotalCacheReadTokens > 0 {
		hint := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("(Không có phí bảo hiểm cho bộ nhớ đệm tự động)")
		b.WriteString(renderField("Khối lượng ghi bộ nhớ đệm", "0 "+hint))
	}

	if len(snap.CachePerAgent) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(colorDim).
			Render(strings.Repeat("·", max(8, width-12))))
		b.WriteString("\n")
		for _, a := range snap.CachePerAgent {
			b.WriteString(renderCacheAgentLine(a, width))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// colorPercent chuyển đổi phần trăm thành chuỗi sau khi tô màu theo tỷ lệ trúng. Nó chỉ được sử dụng cho các cột giá trị.
func colorPercent(p float64) string {
	return lipgloss.NewStyle().Foreground(cacheHitColor(p)).Bold(true).
		Render(formatPercent(p))
}

// renderCacheAgentLine Hiển thị một dòng vai trò duy nhất: vai trò + tốc độ truy cập + số lần đọc bộ nhớ đệm/tổng ​​đầu vào.
//
// Việc đưa ra cả tử số và mẫu số (cacheRead/input) cho phép người dùng kiểm tra nhanh nguồn của tốc độ truy cập.
// Cũng có thể xác định dữ liệu sán lá "tỷ lệ phần trăm cao nhưng mẫu nhỏ" (ví dụ: độ tin cậy 100% / 1k thấp hơn 80% / 300k).
//
// Tỷ lệ phần trăm ưu tiên cho giá trị trạng thái ổn định của cửa sổ trượt; khi không có mẫu nào trong cửa sổ, nó sẽ quay trở lại trạng thái tích lũy. Đây là vị trí duy nhất trong toàn bộ cột bên trái sử dụng "/".
// Ngữ nghĩa là cụ thể (dấu phân chia toán học: số lần truy cập bộ đệm/tổng ​​đầu vào) và sẽ không bị nhầm lẫn với các dấu phân cách khác.
//
// Ba trạng thái:
//
//	Chưa được bật "WRITER chưa được bật"
//	Đã bật "VĂN BẢN 85% · 323k / 394k"
//	Không có bộ đệm rõ ràng là "không được bật", không trộn vào 0/0 để cản trở việc giải thích
func renderCacheAgentLine(a host.AgentCacheStat, width int) string {
	// Tên vai trò hoàn toàn giống với khu vực "Vai trò đang chạy"; Chiều rộng là 12 để làm ĐIỀU PHỐI VIÊN dài nhất
	// 1 cột dấu cách ở cuối vẫn có thể được giữ lại để phân tách và các vai trò khác sẽ tự động được điền vào bên phải.
	roleStyle := lipgloss.NewStyle().Foreground(eventAgentColor(a.Role)).Width(12)
	role := roleStyle.Render(agentDisplayName(a.Role))

	if !a.CacheCapable {
		dim := lipgloss.NewStyle().Foreground(colorDim).Italic(true)
		_ = width
		return role + dim.Render("Chưa bật")
	}

	// Tỷ lệ trúng ở trạng thái ổn định được ưu tiên; khi không có mẫu nào trong cửa sổ, nó sẽ quay trở lại trạng thái tích lũy.
	hit := cacheHitRate(a.RecentCacheRead, a.RecentInput)
	if a.RecentSamples == 0 || a.RecentInput == 0 {
		hit = cacheHitRate(a.CacheRead, a.Input)
	}
	// Tỷ lệ phần trăm được cố định ở độ rộng 4 cột ("100%") để ngăn cột đọc nhảy sang trái và phải trong khoảng "5%" và "85%".
	pctCell := lipgloss.NewStyle().Width(4).
		Render(colorPercent(hit))

	// Giá trị đọc tích lũy/đầu vào tích lũy - Ngay cả khi tỷ lệ phần trăm ở trên là giá trị cửa sổ trượt, thì tử số và mẫu số đều là tích lũy, bởi vì
	// “Thấy quy mô” là điểm hấp dẫn chính của chuyên mục này; riêng tỷ lệ phần trăm có thể cung cấp tín hiệu ở trạng thái ổn định.
	tokens := lipgloss.NewStyle().Foreground(colorDim).Render(
		" · " + formatTokensCompact(a.CacheRead) + " / " + formatTokensCompact(a.Input))
	_ = width
	return role + pctCell + tokens
}

// cacheHitRate được chia trực tiếp cho tỷ lệ phần trăm khi đầu vào đã chứa ngữ nghĩa cacheRead.
// Trả về 0 khi nhập == 0 để tránh các lần truy cập sai.
func cacheHitRate(cacheRead, input int) float64 {
	if input <= 0 {
		return 0
	}
	return float64(cacheRead) / float64(input) * 100
}

// cacheHitColor tỷ lệ trúng màu: ≥50% xanh lục / 20–50% vàng / <20% đỏ.
// Sử dụng hướng ngược lại với việc sử dụng ngữ cảnh: tốc độ truy cập bộ đệm càng cao thì càng tốt cho sức khỏe.
func cacheHitColor(percent float64) lipgloss.AdaptiveColor {
	switch {
	case percent >= 50:
		return colorSuccess
	case percent >= 20:
		return colorReview
	default:
		return colorError
	}
}

func formatPercent(p float64) string {
	if p <= 0 {
		return "0%"
	}
	if p < 10 {
		return fmt.Sprintf("%.1f%%", p)
	}
	return fmt.Sprintf("%.0f%%", p)
}

// formatTokensCompact hiển thị số mã thông báo thành dạng thu gọn, chẳng hạn như "8,2k" / "1,4M".
// Hữu ích cho các dòng thu hẹp trên mỗi vai trò để tránh bị lấn át bởi kiểu dấu phẩy của formatNumber.
func formatTokensCompact(n int) string {
	if n <= 0 {
		return "0"
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func renderContextSidebar(snap host.UISnapshot, width int) string {
	if snap.ContextWindow <= 0 && snap.ContextStrategy == "" && snap.ContextScope == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(renderContextUsageField("bối cảnh chính", snap.ContextPercent, snap.ContextTokens, snap.ContextWindow))
	if strategy := contextStrategyLabel(snap.ContextStrategy); strategy != "" {
		b.WriteString(renderField("Chiến lược gần đây", truncate(strategy, max(8, width-12))))
	}
	if scope := contextScopeLabel(snap.ContextScope); scope != "" {
		b.WriteString(renderField("Chế độ xem hiện tại", scope))
	}
	if snap.ContextSummaryCount > 0 {
		b.WriteString(renderField("bản tóm tắt", fmt.Sprintf("%d mặt hàng", snap.ContextSummaryCount)))
	}
	if snap.ContextActiveMessages > 0 {
		b.WriteString(renderField("Số lượng tin nhắn", fmt.Sprintf("%d", snap.ContextActiveMessages)))
	}
	if snap.ContextCompactedCount > 0 || snap.ContextKeptCount > 0 {
		b.WriteString(renderField("được viết lại gần đây", fmt.Sprintf("%d → %d", snap.ContextCompactedCount, snap.ContextKeptCount)))
	}
	return b.String()
}

func contextScopeLabel(scope string) string {
	switch scope {
	case "baseline":
		return "đường cơ sở"
	case "projected":
		return "phép chiếu"
	case "recovered":
		return "hồi phục"
	case "committed":
		return "Đã gửi"
	case "skipped":
		return "ngắt mạch bị bỏ qua"
	default:
		return scope
	}
}

func contextStrategyLabel(strategy string) string {
	switch strategy {
	case "":
		return ""
	case "tool_result_microcompact":
		return "Kết quả công cụ nén vi mô"
	case "light_trim":
		return "cây trồng nhẹ"
	case "full_summary":
		return "Tóm tắt đầy đủ"
	default:
		return strategy
	}
}

func agentDisplayName(name string) string {
	return strings.ToUpper(name)
}

func agentTaskLine(agent host.AgentSnapshot) string {
	if agent.TaskKind != "" {
		return taskKindLabel(agent.TaskKind)
	}
	if agent.Summary != "" {
		return agent.Summary
	}
	return ""
}

func agentContextLine(agent host.AgentSnapshot) string {
	ctx := agent.Context
	if ctx.ContextWindow <= 0 || ctx.Tokens <= 0 {
		return ""
	}
	percentColor := contextPercentColor(ctx.Percent)
	percentStr := lipgloss.NewStyle().Foreground(percentColor).Render(fmt.Sprintf("ctx %.0f%%", ctx.Percent))
	parts := []string{percentStr}
	if scope := contextScopeLabel(ctx.Scope); scope != "" {
		parts = append(parts, scope)
	}
	if strategy := contextStrategyLabel(ctx.Strategy); strategy != "" {
		parts = append(parts, strategy)
	}
	return strings.Join(parts, " · ")
}

func agentStateRank(state string) int {
	switch state {
	case "running":
		return 0
	case "failed":
		return 1
	default:
		return 2
	}
}

func agentOrder(name string) int {
	switch {
	case strings.HasPrefix(name, "architect"):
		return 0
	case name == "coordinator":
		return 1
	case name == "editor":
		return 2
	case name == "writer":
		return 3
	default:
		return 9
	}
}

func agentStateLabel(state string) string {
	switch state {
	case "running":
		return "Đang chạy"
	case "failed":
		return "bất thường"
	case "idle":
		return "Chế độ chờ"
	default:
		return state
	}
}

func agentStateIcon(state string) string {
	switch state {
	case "running":
		return "●"
	case "failed":
		return "×"
	default:
		return "·"
	}
}

func taskStatusColor(status string) lipgloss.AdaptiveColor {
	switch status {
	case "running":
		return colorSuccess
	case "queued":
		return colorMuted
	case "failed", "canceled":
		return colorError
	case "succeeded":
		return colorSuccess
	default:
		return colorDim
	}
}

func taskKindLabel(kind string) string {
	switch kind {
	case "foundation_plan":
		return "quy hoạch cơ bản"
	case "chapter_write":
		return "Viết chương"
	case "chapter_review":
		return "Đánh giá chương"
	case "chapter_rewrite":
		return "Viết lại chương"
	case "chapter_polish":
		return "Đánh bóng chương"
	case "arc_expand":
		return "Mở rộng vòng cung"
	case "volume_append":
		return "Lên kế hoạch cho tập tiếp theo"
	case "steer_apply":
		return "can thiệp điều trị"
	case "coordinator_decision":
		return "Phối hợp thăng tiến"
	default:
		return kind
	}
}

// renderEventContent hiển thị danh sách các sự kiện thành luồng sự kiện phân cấp.
// DISPATCH Là một tiêu đề cấp cao nhất, các công cụ tác nhân phụ được hiển thị thụt vào, tạo thành một cây lập kế hoạch rõ ràng.
// spinnerFrame được sử dụng để hiển thị các biểu tượng động cho các hàng "đang xử lý" (được đồng bộ hóa với công cụ quay vòng thanh trên cùng).
func renderEventContent(events []host.Event, width, spinnerFrame int) string {
	var b strings.Builder
	for i, ev := range events {
		b.WriteString(renderEventLine(ev, width, spinnerFrame))
		if i < len(events)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// Khung quay được sử dụng bởi các sự kiện lớp cuộc gọi đang diễn ra (bubbles.Spinner.Dot, độc lập với thanh MiniDot trên cùng).
var eventRunningFrames = toolSpinnerFrames

func runningSpinner(frame int) string {
	return eventRunningFrames[frame%len(eventRunningFrames)]
}

func renderEventLine(ev host.Event, width, spinnerFrame int) string {
	tsStr := lipgloss.NewStyle().Foreground(colorDim).Render(ev.Time.Format("15:04:05"))
	indent := ""
	if ev.Depth > 0 {
		indent = "  "
	}
	maxSumW := max(20, width-12-ev.Depth*2)

	running := ev.Running()
	durStr := renderEventDuration(ev.Duration)

	switch {
	case ev.Category == "DISPATCH":
		// Ba trạng thái: đang tiến hành (trọng âm + đậm) / không thành công (đỏ ✕) / đã hoàn thành (xanh ✓)
		var icon string
		switch {
		case running:
			icon = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(runningSpinner(spinnerFrame))
		case ev.Failed:
			icon = lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("✕")
		default:
			icon = lipgloss.NewStyle().Foreground(colorSuccess).Render("✓")
		}
		sum := renderDispatchSummary(ev.Summary, maxSumW)
		if running {
			// Đang trong quá trình giữ nguyên nhưng in đậm
			sum = lipgloss.NewStyle().Bold(true).Render(sum)
		}
		line := tsStr + " " + icon + " " + sum
		if !running {
			line += durStr
		}
		return line

	case ev.Category == "DONE":
		// Tương thích với dữ liệu phát lại cũ; quy trình mới không còn tạo ra các sự kiện độc lập DONE
		icon := lipgloss.NewStyle().Foreground(colorSuccess).Render("✓")
		color := eventAgentColor(ev.Agent)
		name := lipgloss.NewStyle().Foreground(color).Render(agentDisplayName(ev.Agent))
		return tsStr + " " + icon + " " + name + durStr

	case ev.Category == "TOOL" && ev.Depth == 0:
		// công cụ riêng của điều phối viên
		var icon, sum string
		switch {
		case running:
			icon = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(runningSpinner(spinnerFrame))
			sum = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(truncate(ev.Summary, maxSumW))
		case ev.Failed:
			icon = lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("✕")
			sum = lipgloss.NewStyle().Foreground(colorError).Render(truncate(ev.Summary, maxSumW))
		default:
			icon = lipgloss.NewStyle().Foreground(colorTool).Render("◇")
			sum = lipgloss.NewStyle().Foreground(colorTool).Render(truncate(ev.Summary, maxSumW))
		}
		line := tsStr + " " + icon + " " + sum
		if !running {
			line += durStr
		}
		return line

	case ev.Category == "TOOL":
		// công cụ nội bộ của tác nhân phụ (Độ sâu=1)
		var icon, sum string
		switch {
		case running:
			icon = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(runningSpinner(spinnerFrame))
			sum = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(truncate(ev.Summary, maxSumW))
		case ev.Failed:
			icon = lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("✕")
			sum = lipgloss.NewStyle().Foreground(colorError).Render(truncate(ev.Summary, maxSumW))
		default:
			icon = lipgloss.NewStyle().Foreground(colorDim).Render("├")
			sum = lipgloss.NewStyle().Foreground(colorMuted).Render(truncate(ev.Summary, maxSumW))
		}
		line := tsStr + " " + indent + icon + " " + sum
		if !running {
			line += durStr
		}
		return line

	case ev.Category == "ERROR":
		icon := lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("✕")
		errStyle := lipgloss.NewStyle().Foreground(colorError)
		lines := wrapStreamText(ev.Summary, maxSumW)
		first := tsStr + " " + indent + icon + " " + errStyle.Render(lines[0])
		pad := strings.Repeat(" ", 10+len(indent))
		for _, l := range lines[1:] {
			first += "\n" + pad + errStyle.Render(l)
		}
		if durStr != "" {
			first += durStr
		}
		return first

	case ev.Category == "SYSTEM":
		icon := lipgloss.NewStyle().Foreground(colorAccent).Render("⚙")
		sumColor := colorMuted
		if ev.Level == "warn" {
			sumColor = colorAccent
		}
		sum := lipgloss.NewStyle().Foreground(sumColor).Render(truncate(ev.Summary, maxSumW))
		return tsStr + " " + indent + icon + " " + sum

	case ev.Category == "USER":
		// Văn bản Chỉ đạo / Tiếp tục do người dùng gửi trong hộp nhập được lặp lại; nó được mở bằng ⚙ của HỆ THỐNG và ✎ được sử dụng để biểu thị "đầu vào".
		// Màu sắc được phân tách bằng colorAccent2 (màu ngọc lam) và màu vàng của HỆ THỐNG để tránh đọc nhầm thành thông báo hệ thống.
		icon := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render("✎")
		sum := lipgloss.NewStyle().Foreground(colorAccent2).Render(truncate(ev.Summary, maxSumW))
		return tsStr + " " + indent + icon + " " + sum

	case ev.Category == "CONTEXT" || ev.Category == "COMPACT":
		icon := lipgloss.NewStyle().Foreground(colorContext).Render("⚙")
		sumColor := colorContext
		if ev.Level == "debug" {
			sumColor = colorMuted
		}
		sum := lipgloss.NewStyle().Foreground(sumColor).Render(truncate(ev.Summary, maxSumW))
		return tsStr + " " + indent + icon + " " + sum

	default:
		// Các danh mục đã biết tuân theo màu được ánh xạ; các danh mục không xác định tuân theo nền trước mặc định của thiết bị đầu cuối để tránh buộc colorText.
		if color, ok := categoryColors[ev.Category]; ok {
			icon := lipgloss.NewStyle().Foreground(color).Render("·")
			sum := lipgloss.NewStyle().Foreground(color).Render(truncate(ev.Summary, maxSumW))
			return tsStr + " " + indent + icon + " " + sum
		}
		icon := lipgloss.NewStyle().Foreground(colorDim).Render("·")
		return tsStr + " " + indent + icon + " " + truncate(ev.Summary, maxSumW)
	}
}

// renderDispatchSummary Kết xuất DISPATCH Tóm tắt: Tên đặc vụ sử dụng màu sắc vai trò và nhiệm vụ sử dụng màu sáng.
func renderDispatchSummary(summary string, maxW int) string {
	agentName := summary
	taskPart := ""
	if idx := strings.Index(summary, "（"); idx > 0 {
		agentName = summary[:idx]
		taskPart = summary[idx:]
	}
	displayName := agentDisplayName(agentName)
	color := eventAgentColor(agentName)
	nameW := lipgloss.Width(displayName)
	if nameW >= maxW {
		return lipgloss.NewStyle().Foreground(color).Bold(true).Render(truncate(displayName, maxW))
	}
	result := lipgloss.NewStyle().Foreground(color).Bold(true).Render(displayName)
	if taskPart != "" {
		remaining := maxW - nameW
		if remaining > 2 {
			result += lipgloss.NewStyle().Foreground(colorMuted).Render(truncate(taskPart, remaining))
		}
	}
	return result
}

// eventAgentColor trả về màu chủ đề tương ứng với vai trò Đại lý.
func eventAgentColor(agent string) lipgloss.AdaptiveColor {
	switch {
	case strings.HasPrefix(agent, "architect"):
		return colorAccent2
	case agent == "writer":
		return colorTool
	case agent == "editor":
		return colorReview
	default:
		return colorAccent
	}
}

// renderEventDuration Hiển thị Thời lượng dưới dạng chú thích khung màu sáng. Giá trị 0 trả về giá trị trống.
func renderEventDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return " " + lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("("+formatDuration(d)+")")
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

func renderEventActivity(snap host.UISnapshot, frame, width int) string {
	if !snap.IsRunning {
		return ""
	}
	return renderEventSparkle(frame, width)
}

var sparkleFrames = []string{
	"✦  ·   ✧   ·  ✦",
	"·  ✧   ·  ✦   ·",
	"  ✧   ·  ✦   · ",
	"   ·  ✦   ·  ✧ ",
	"✧   ·  ✦  ·   ✧",
	" ·  ✧   ·  ✦  ·",
	"✦   ·  ✧   ·  ✦",
	" ·  ✦   ·  ✧   ",
}

func renderEventSparkle(frame, width int) string {
	pattern := sparkleFrames[frame%len(sparkleFrames)]

	var b strings.Builder
	for _, ch := range pattern {
		switch ch {
		case '✦':
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#d4a21a")).Bold(true).Render("✦"))
		case '✧':
			b.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).Render("✧"))
		case '·':
			b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("·"))
		default:
			b.WriteRune(ch)
		}
	}
	_ = width
	return " " + b.String()
}

// renderEventFlowViewport bao bọc bảng luồng sự kiện kết xuất bằng một khung nhìn.
func renderEventFlowViewport(vp viewport.Model, width, height int, focused bool) string {
	// thanh tiêu đề
	titleColor := colorDim
	if focused {
		titleColor = colorAccent
	}
	title := lipgloss.NewStyle().Foreground(titleColor).Render(":: luồng sự kiện")
	lineW := width - lipgloss.Width(title) - 4
	if lineW < 0 {
		lineW = 0
	}
	separator := lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("─", lineW))
	header := " " + title + " " + separator

	vpH := height - 1
	if vpH < 1 {
		vpH = 1
	}
	style := lipgloss.NewStyle().
		Width(width).
		Height(vpH).
		Padding(0, 1)

	return header + "\n" + style.Render(vp.View())
}

// renderStreamPanel hiển thị bảng đầu ra phát trực tuyến (nửa dưới của cột giữa).
func renderStreamPanel(vp viewport.Model, width, height int, focused, running bool, frame int) string {
	// Tách biệt thanh tiêu đề (luôn bắt mắt): in đậm tiền tố thanh dọc + luôn in đậm + màu nhấn để tránh xung đột với màu xám nhạt in nghiêng của suy nghĩ
	// Một gạch chân bổ sung được thêm vào khi tập trung để phân biệt trạng thái tiêu điểm.
	titleStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Underline(focused)
	title := titleStyle.Render("▍Đầu ra thời gian thực")
	if running {
		status := renderStreamActivity(frame)
		title += " " + status
	}
	lineW := width - lipgloss.Width(title) - 4
	if lineW < 0 {
		lineW = 0
	}
	separator := lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("─", lineW))
	header := " " + title + " " + separator

	// Nội dung khung nhìn (chiều cao bao gồm dòng tiêu đề, chiều cao thực tế của khung nhìn cần giảm 1).
	// vpStyle bên ngoài không có Foreground - màu của văn bản chương được xác định bởi renderChapterBlock bên trong
	// ống contentStyle (mặc định thiết bị đầu cuối có đáy màu nâu đậm / đáy tối). Nếu Foreground được thêm vào lớp ngoài thì lớp dưới sẽ sáng
	// Khối lập kế hoạch tác nhân (✻ nhãn vàng + lục lam) theo chủ đề sẽ được "ép" bằng màu nâu sẫm thành màu văn bản bình thường.
	vpH := height - 1
	if vpH < 1 {
		vpH = 1
	}
	vpStyle := lipgloss.NewStyle().
		Width(width).
		Height(vpH).
		Padding(0, 1)

	return header + "\n" + vpStyle.Render(vp.View())
}

var streamCursorFrames = []string{"·", "✢", "✳", "✶", "✻", "✽"}

func renderStreamCursor(frame int) string {
	f := frame % len(streamCursorFrames)
	var dots [3]string
	for i := range 3 {
		dots[i] = streamCursorFrames[(f+i)%len(streamCursorFrames)]
	}
	trail := dots[0] + " " + dots[1] + " " + dots[2]
	return "\n" + lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(trail)
}

var streamActivityFrames = [][2]string{
	{"✦", "✧"},
	{"✦", "✧"},
	{"✧", "✦"},
	{"✧", "✦"},
	{"✦", "✧"},
	{"✦", "✧"},
	{"✧", "✦"},
	{"✧", "✦"},
}

func renderStreamActivity(frame int) string {
	pair := streamActivityFrames[frame%len(streamActivityFrames)]
	major := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(pair[0])
	minor := lipgloss.NewStyle().Foreground(colorAccent2).Render(pair[1])
	return major + " " + minor
}

// renderStreamContent Hiển thị đầu ra phát trực tuyến thành các khối ngữ nghĩa theo vòng.
// Các khối điều phối đại lý (bắt đầu bằng ▸ hoặc ✻) sử dụng tiêu đề có dấu + chỉ thị mờ; khối văn bản theo màu mặc định của thiết bị đầu cuối.
// Khi con trỏ không trống, nó sẽ được thêm vào cuối, cho biết AI đang xuất ra.
func renderStreamContent(rounds []string, width int, cursor string) string {
	if width < 24 {
		width = 24
	}

	var blocks []string
	for _, round := range rounds {
		text := strings.TrimSpace(round)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "▸") || strings.HasPrefix(text, "✻") {
			blocks = append(blocks, renderAgentBlock(text, width))
		} else {
			blocks = append(blocks, renderChapterBlock(text, width))
		}
	}
	result := strings.Join(blocks, "\n\n")
	if cursor != "" {
		result += cursor
	}
	return result
}

// renderAgentBlock hiển thị khối lập kế hoạch Tác nhân: biểu tượng + tiêu đề + dải phân cách + hướng dẫn tác vụ.
//
// Nhãn sử dụng colorAccent2 green + Bold + Underline nhấn mạnh ba lần - trước colorAccent
// Vàng + In đậm quá gần với màu sắc Dòng suy nghĩ màu xám mờ trên nền tối và không thể phân biệt được mức độ ưu tiên. Màu xanh lá cây là màu mát mẻ,
// Nó hoàn toàn khác với màu xám ấm áp được Thought sử dụng về mặt sắc độ; Underline ổn định và hiệu quả ở mọi terminal, tốt hơn Bold
// Neo hình ảnh đáng tin cậy hơn. Biểu tượng ✻ lần lượt sử dụng màu vàng làm điểm neo để tạo thành sự tương phản hai màu với nhãn.
func renderAgentBlock(text string, width int) string {
	headerLine, body, _ := strings.Cut(text, "\n")

	iconStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Underline(true)

	// Tách biểu tượng tiền tố (✻ hoặc ▸) và nhãn văn bản rồi tô màu chúng riêng biệt; định dạng cũ không có biểu tượng vẫn đơn sắc.
	var headerStyled string
	if first, rest, ok := strings.Cut(headerLine, " "); ok && (first == "✻" || first == "▸") {
		headerStyled = iconStyle.Render(first) + " " + labelStyle.Render(rest)
	} else {
		headerStyled = labelStyle.Render(headerLine)
	}

	// Dòng tiêu đề + dòng phân cách (dòngW sử dụng chiều rộng trực quan của headerLine thay vì chiều rộng byte được hiển thị)
	titleW := lipgloss.Width(headerLine)
	lineW := max(0, width-titleW-1)
	header := headerStyled +
		" " + lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("─", lineW))

	var b strings.Builder
	b.WriteString(header)

	// Hướng dẫn nhiệm vụ: tô màu mờ, thụt 2 dấu cách; để lại một dòng trống giữa tiêu đề và tiêu đề để tránh dính hình ảnh.
	body = strings.TrimSpace(body)
	if body != "" {
		taskStyle := lipgloss.NewStyle().Foreground(colorMuted)
		lines := wrapStreamText(body, max(16, width-6))
		b.WriteString("\n\n")
		for i, line := range lines {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(taskStyle.Render("  " + line))
		}
	}
	return b.String()
}

// renderChapterBlock hiển thị khối văn bản và tự động phân biệt nội dung suy nghĩ và văn bản chương.
// Nội dung tư duy (các đoạn được đánh dấu bởi ThoughtSep) được in nghiêng bằng colorDim; bodyTextColor được sử dụng cho văn bản chương:
// Nền tối kế thừa nền trước mặc định của thiết bị đầu cuối và nền sáng sử dụng màu nâu sẫm để giữ tông màu ấm.
func renderChapterBlock(text string, width int) string {
	contentStyle := lipgloss.NewStyle().Foreground(bodyTextColor)
	thinkStyle := lipgloss.NewStyle().Foreground(colorDim).Italic(true)
	wrapW := max(16, width-4)

	// Chia theo ThoughtSep: đoạn văn lẻ là suy nghĩ, đoạn văn chẵn là văn bản
	// Định dạng: [Văn bản] \x02 [Suy nghĩ] [Văn bản] \x02 [Suy nghĩ] ...
	parts := strings.Split(text, utils.ThinkingSep)

	var b strings.Builder
	for i, part := range parts {
		part = strings.TrimRight(part, " \n")
		if part == "" {
			continue
		}
		isThinking := i > 0 && i%2 != 0 // Các phân đoạn đánh số lẻ sau ThoughtSep là Suy nghĩ

		style := contentStyle
		if isThinking {
			style = thinkStyle
		}

		lines := wrapStreamText(part, wrapW)
		for j, line := range lines {
			if b.Len() > 0 && j == 0 {
				b.WriteString("\n\n") // Dòng trống giữa các đoạn văn: Để lại khoảng cách trực quan giữa suy nghĩ và văn bản
			} else if j > 0 {
				b.WriteString("\n")
			}
			b.WriteString(style.Render(line))
		}
	}
	return b.String()
}

func wrapStreamText(text string, width int) []string {
	if width < 8 {
		return []string{text}
	}

	var out []string
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(raw) == "" {
			out = append(out, "")
			continue
		}
		prefix, rest, nextPrefix := parseWrapPrefix(raw)
		wrapped := wrapRunes(rest, max(4, width-lipgloss.Width(prefix)))
		for i, line := range wrapped {
			if i == 0 {
				out = append(out, prefix+line)
				continue
			}
			out = append(out, nextPrefix+line)
		}
	}
	return out
}

func parseWrapPrefix(line string) (prefix, content, nextPrefix string) {
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	trimmed := strings.TrimSpace(line)

	switch {
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "• "):
		prefix = indent + trimmed[:2]
		content = strings.TrimSpace(trimmed[2:])
		nextPrefix = indent + "  "
		return prefix, content, nextPrefix
	case orderedListPrefix(trimmed) != "":
		marker := orderedListPrefix(trimmed)
		prefix = indent + marker
		content = strings.TrimSpace(strings.TrimPrefix(trimmed, marker))
		nextPrefix = indent + strings.Repeat(" ", lipgloss.Width(marker))
		return prefix, content, nextPrefix
	case strings.HasPrefix(trimmed, "```"):
		return indent, trimmed, indent
	default:
		return indent, trimmed, indent
	}
}

func orderedListPrefix(line string) string {
	end := strings.Index(line, ". ")
	if end <= 0 {
		return ""
	}
	for _, r := range line[:end] {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return line[:end+2]
}

func wrapRunes(text string, width int) []string {
	if text == "" {
		return []string{""}
	}
	if width < 2 {
		return []string{text}
	}

	var lines []string
	var current strings.Builder
	currentWidth := 0

	for _, r := range text {
		rw := lipgloss.Width(string(r))
		if currentWidth > 0 && currentWidth+rw > width {
			lines = append(lines, strings.TrimRight(current.String(), " "))
			current.Reset()
			currentWidth = 0
			if r == ' ' {
				continue
			}
		}
		current.WriteRune(r)
		currentWidth += rw
	}
	if current.Len() > 0 {
		lines = append(lines, strings.TrimRight(current.String(), " "))
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// phác thảoGridThreshold Ngưỡng chương chuyển đổi phác thảo cho nhiều cột.
// Giới hạn trên của cấp ngắn là 25 chương và 20 chương sau có thể nằm gọn trong một cột trên một màn hình và có thể giữ lại biểu tượng "đang tiến hành";
// Sau khi chế độ xếp lớp dài được cuộn và mở rộng, n sẽ tự nhiên vượt quá 20 và chuyển sang nhiều cột một cách trơn tru.
const outlineGridThreshold = 20

// renderOutlineSection chọn bố cục dựa trên số chương: ít nhất là một cột (có biểu tượng "đang tiến hành"), nhiều như một lưới nhiều cột.
func renderOutlineSection(snap host.UISnapshot, contentW int) string {
	if len(snap.Outline) < outlineGridThreshold {
		return renderOutlineList(snap, contentW)
	}
	return renderOutlineGrid(snap, contentW)
}

// renderOutlineList Danh sách chương một cột (dành cho truyện ngắn). Với logo “Đang tiến hành” ở cuối mỗi dòng, nhịp đọc dọc gần với mục lục hơn.
func renderOutlineList(snap host.UISnapshot, contentW int) string {
	var b strings.Builder
	for _, e := range snap.Outline {
		ch := fmt.Sprintf("%2d", e.Chapter)
		var marker, chStyle string
		titleStyle := cardContentStyle
		switch {
		case snap.CompletedCount >= e.Chapter:
			marker = lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
			chStyle = lipgloss.NewStyle().Foreground(colorDim).Render(ch)
		case snap.InProgressChapter == e.Chapter:
			marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("▸")
			chStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(ch)
			titleStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
		default:
			marker = lipgloss.NewStyle().Foreground(colorDim).Render("○")
			chStyle = lipgloss.NewStyle().Foreground(colorDim).Render(ch)
			titleStyle = lipgloss.NewStyle().Foreground(colorMuted)
		}
		title := truncate(e.Title, contentW-6)
		line := marker + chStyle + " " + titleStyle.Render(title)
		if snap.InProgressChapter == e.Chapter {
			line += lipgloss.NewStyle().Foreground(colorAccent).Italic(true).Render(" đang tiến hành")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// renderOutlineGrid điền các chương phác thảo vào lưới nhiều cột ở "mức độ ưu tiên cột" để tránh lượng lớn khoảng trắng trong một cột duy nhất trên màn hình rộng.
// Số lượng cột thích ứng theo nội dungW (1-4), số chương trong cột tăng dần (“đọc một cột trước khi đọc cột tiếp theo”).
// Sự đánh đổi so với bố cục một cột: Bỏ logo "đang tiến hành" ở cuối - logo nhiều cột phá vỡ sự liên kết cột,
// Và dấu ▸ + vàng + "Viết Chương N" ở thanh tổng quan bên trái đã ghi rõ thông tin đang diễn ra.
func renderOutlineGrid(snap host.UISnapshot, contentW int) string {
	n := len(snap.Outline)
	if n == 0 {
		return ""
	}
	chNumW := 2
	titleW := 0
	for _, e := range snap.Outline {
		if w := len(strconv.Itoa(e.Chapter)); w > chNumW {
			chNumW = w
		}
		if w := lipgloss.Width(e.Title); w > titleW {
			titleW = w
		}
	}
	// Giới hạn trên của độ rộng tiêu đề là 14 (khoảng 7 ký tự tiếng Trung); đôi khi các tiêu đề dài bị cắt bớt để ngăn một hoặc hai tiêu đề dài lấp đầy toàn bộ ô.
	if titleW > 14 {
		titleW = 14
	} else if titleW < 4 {
		titleW = 4
	}
	cellW := 3 + chNumW + titleW // điểm đánh dấu(1) + dấu cách(1) + số chương + dấu cách(1) + tiêu đề
	gutter := 4
	cols := (contentW + gutter) / (cellW + gutter)
	if cols < 1 {
		cols = 1
	} else if cols > 4 {
		cols = 4
	}
	rows := (n + cols - 1) / cols

	var b strings.Builder
	cellStyle := lipgloss.NewStyle().Width(cellW)
	gutterStr := strings.Repeat(" ", gutter)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			idx := c*rows + r
			if idx >= n {
				break
			}
			cell := renderOutlineCell(snap.Outline[idx], snap, chNumW, titleW)
			// Khi có ô ở các cột tiếp theo, nhấn cellW để hoàn thành + gutter; nếu không, ô hiện tại sẽ không được hoàn thành ở cuối dòng.
			if c < cols-1 && (c+1)*rows+r < n {
				b.WriteString(cellStyle.Render(cell))
				b.WriteString(gutterStr)
			} else {
				b.WriteString(cell)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderOutlineCell hiển thị một ô chương duy nhất: đã hoàn thành (xanh ●)/đang tiến hành (vàng ▸)/chưa bắt đầu (tối ○).
func renderOutlineCell(e host.OutlineSnapshot, snap host.UISnapshot, chNumW, titleW int) string {
	chStr := fmt.Sprintf("%*d", chNumW, e.Chapter)
	title := truncateWidth(e.Title, titleW)
	var marker, chRendered, titleRendered string
	switch {
	case snap.CompletedCount >= e.Chapter:
		marker = lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
		chRendered = lipgloss.NewStyle().Foreground(colorDim).Render(chStr)
		titleRendered = cardContentStyle.Render(title)
	case snap.InProgressChapter == e.Chapter:
		marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("▸")
		chRendered = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(chStr)
		titleRendered = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title)
	default:
		marker = lipgloss.NewStyle().Foreground(colorDim).Render("○")
		chRendered = lipgloss.NewStyle().Foreground(colorDim).Render(chStr)
		titleRendered = lipgloss.NewStyle().Foreground(colorMuted).Render(title)
	}
	return marker + " " + chRendered + " " + titleRendered
}

// truncateWidth cắt ngắn theo "chiều rộng hình ảnh" (ký tự tiếng Trung được tính là 2 cột) và có cùng nguồn gốc với lipgloss.Width.
// Việc cắt ngắn thông thường được tính theo rune và sẽ được cắt ngắn theo chiều rộng gấp đôi đối với tiếng Trung. Nó không thể được sử dụng khi cần căn chỉnh cột.
func truncateWidth(s string, maxW int) string {
	if lipgloss.Width(s) <= maxW {
		return s
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > maxW {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String()
}

// renderDetailContent xây dựng nội dung của bảng chi tiết ở bên phải.
// Trước tiên, hiển thị các cài đặt cơ bản (phác thảo, vai trò), sau đó là thông tin thời gian chạy (cam kết, đánh giá, v.v.).
func renderDetailContent(snap host.UISnapshot, contentW int) string {
	var b strings.Builder

	// phác thảo
	if len(snap.Outline) > 0 {
		outlineHeader := "::Phác thảo"
		if snap.Layered {
			outlineHeader = fmt.Sprintf(":: Outline (%s · Dynamic Programming Outline)", snap.CurrentVolumeArc)
		}
		b.WriteString(panelTitleStyle.Render(outlineHeader))
		b.WriteString("\n")
		b.WriteString(renderOutlineSection(snap, contentW))
		// Mẹo lập kế hoạch lăn
		compassStyle := lipgloss.NewStyle().Foreground(colorDim).Italic(true)
		if snap.Layered {
			if snap.NextVolumeTitle != "" {
				b.WriteString(compassStyle.Render("  ┄Tập tiếp theo:" + snap.NextVolumeTitle))
				b.WriteString("\n")
			}
			b.WriteString(compassStyle.Render("  ··· Các chương tiếp theo sẽ được tạo tự động khi quá trình tạo diễn ra"))
			b.WriteString("\n")
			if snap.CompassDirection != "" {
				direction := fmt.Sprintf("  → Kết thúc: %s", snap.CompassDirection)
				if snap.CompassScale != "" {
					direction += "（" + snap.CompassScale + "）"
				}
				b.WriteString(compassStyle.Render(truncate(direction, contentW)))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Vai trò
	if len(snap.Characters) > 0 {
		b.WriteString(panelTitleStyle.Render(":: Vai trò"))
		b.WriteString("\n")
		for _, c := range snap.Characters {
			b.WriteString(cardContentStyle.Render("· " + truncate(c, contentW-2)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Hệ sinh thái nhân vật phụ: tổng số nhân vật phụ đã xuất hiện + top 5 nhân vật hoạt động gần đây nhất
	if snap.SupportingCount > 0 {
		b.WriteString(panelTitleStyle.Render(":: Vai trò hỗ trợ sinh thái"))
		b.WriteString("\n")
		b.WriteString(cardContentStyle.Render(truncate(fmt.Sprintf("Đã xuất hiện: %d", snap.SupportingCount), contentW)))
		b.WriteString("\n")
		for _, name := range snap.RecentSupporting {
			b.WriteString(cardContentStyle.Render("· " + truncate(name, contentW-2)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// tiền đề
	if snap.Premise != "" {
		b.WriteString(panelTitleStyle.Render(":: Tiền đề"))
		b.WriteString("\n")
		for _, line := range wrapStreamText(snap.Premise, contentW) {
			b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render(line))
			b.WriteString("\n")
		}
		b.WriteString("\n\n")
	}

	if snap.LastCommitSummary != "" {
		b.WriteString(cardTitleStyle.Render("~ Bài nộp gần đây ~"))
		b.WriteString("\n")
		b.WriteString(cardContentStyle.Render(snap.LastCommitSummary))
		b.WriteString("\n\n")
	}

	if snap.LastReviewSummary != "" {
		b.WriteString(cardTitleStyle.Render("~Đánh giá mới nhất~"))
		b.WriteString("\n")
		b.WriteString(cardContentStyle.Render(snap.LastReviewSummary))
		b.WriteString("\n\n")
	}

	if len(snap.RecentSummaries) > 0 {
		b.WriteString(cardTitleStyle.Render("~ Tóm tắt ~"))
		b.WriteString("\n")
		for _, s := range snap.RecentSummaries {
			b.WriteString(cardContentStyle.Render(truncate(s, contentW)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderDetailPanel hiển thị bảng chi tiết có thể cuộn ở bên phải.
func renderDetailPanel(vp viewport.Model, width, height int, focused bool) string {
	borderColor := colorDim
	if focused {
		borderColor = colorAccent
	}
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxHeight(height).
		Border(baseBorder, false, false, false, true).
		BorderForeground(borderColor).
		Padding(0, 1)

	return style.Render(vp.View())
}

// renderWelcome hiển thị màn hình đầu tiên của trạng thái mới.
func renderWelcome(width, height int, errMsg string, mode startupMode) string {
	// tiêu đề ngắn gọn
	title := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render("A I N O V E L")

	// phụ đề
	subtitle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true).
		Render("AI-Powered Novel Creation Engine")

	// dải phân cách
	divW := 44
	if divW > width-8 {
		divW = width - 8
	}
	divider := lipgloss.NewStyle().Foreground(colorDim).
		Render(strings.Repeat("~", divW))

	// Tính năng nổi bật
	features := []struct{ icon, label, desc string }{
		{">>", "Hợp tác đa mô hình", "Lập kế hoạch kiến ​​trúc/Sáng tạo nhà văn/Đánh giá biên tập viên"},
		{"::", "Phục hồi điểm dừng", "Tự động tiếp tục viết từ tiến trình cuối cùng sau khi gặp sự cố hoặc gián đoạn"},
		{"<>", "can thiệp thời gian thực", "Điều chỉnh hướng cốt truyện bất cứ lúc nào trong quá trình tạo"},
		{"##", "Tiểu thuyết nhiều lớp", "Hỗ trợ tạo biểu mẫu dài với cấu trúc phân cấp tập-cung-chương"},
	}
	iconStyle := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
	featLabelStyle := lipgloss.NewStyle().Foreground(bodyTextColor)
	descStyle := lipgloss.NewStyle().Foreground(colorDim)
	var featLines []string
	for _, f := range features {
		line := iconStyle.Render(f.icon) + " " +
			featLabelStyle.Render(f.label) + "  " +
			descStyle.Render(f.desc)
		featLines = append(featLines, line)
	}
	feats := strings.Join(featLines, "\n")

	// Lời nhắc đầu vào
	prompt := lipgloss.NewStyle().Foreground(bodyTextColor).Render("Nhập yêu cầu mới lạ của bạn bên dưới để bắt đầu viết")

	modeLine := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render("Chế độ hiện tại:" + mode.label() + " · " + mode.subtitle())

	// Ví dụ
	examples := []string{
		"Viết một cuốn tiểu thuyết hồi hộp đô thị dài 12 chương với nhân vật chính là nữ bác sĩ pháp y",
		"Tạo một cuốn tiểu thuyết cổ tích trong đó nhân vật chính từ phàm trần trở thành thăng thiên",
		"Viết một truyện ngắn khoa học viễn tưởng về vấn đề nan giải về đạo đức sau sự thức tỉnh của AI",
	}
	exStyle := lipgloss.NewStyle().Foreground(colorAccent)
	dotStyle := lipgloss.NewStyle().Foreground(colorDim)
	var exLines []string
	for _, ex := range examples {
		exLines = append(exLines, dotStyle.Render("  . ")+exStyle.Render(ex))
	}
	exBlock := strings.Join(exLines, "\n")

	// lắp ráp
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(subtitle)
	b.WriteString("\n\n")
	b.WriteString(divider)
	b.WriteString("\n\n")
	b.WriteString(feats)
	b.WriteString("\n\n")
	b.WriteString(divider)
	b.WriteString("\n\n")
	b.WriteString(modeLine)
	b.WriteString("\n\n")
	b.WriteString(prompt)
	b.WriteString("\n\n")
	b.WriteString(exBlock)
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Italic(true).
		Render("Tab để chuyển chế độ · Nhập vào phần Bắt đầu nhanh để tạo trực tiếp · Nhập vào phần Lập kế hoạch đồng sáng tạo để tham gia cuộc trò chuyện"))

	if errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("! " + errMsg))
	}

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Render(b.String())
}
