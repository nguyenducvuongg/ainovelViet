package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/nguyenducvuongg/ainovelViet/internal/entry/startup"
	"github.com/nguyenducvuongg/ainovelViet/internal/host"
)

type startupMode int

const (
	startupModeQuick startupMode = iota
	startupModeCoCreate
)

func (m startupMode) label() string {
	switch m {
	case startupModeCoCreate:
		return "Đồng sáng tạo quy hoạch"
	default:
		return "bắt đầu nhanh"
	}
}

func (m startupMode) subtitle() string {
	switch m {
	case startupModeCoCreate:
		return "Nói chuyện với AI để làm rõ trước khi bạn bắt đầu tạo"
	default:
		return "Bắt đầu viết trực tiếp bằng một câu"
	}
}

func placeholderForNewMode(mode startupMode) string {
	switch mode {
	case startupModeCoCreate:
		return "Nhập ý tưởng cốt lõi của bạn trước và nhập để bắt đầu đồng sáng tạo với AI"
	default:
		return "Nhập một yêu cầu mới và nhập để bắt đầu viết trực tiếp"
	}
}

func placeholderForCoCreate(state *cocreateState) string {
	if state == nil {
		return placeholderForNewMode(startupModeCoCreate)
	}
	switch {
	case state.awaiting:
		return "AI đang sắp xếp yêu cầu của bạn..."
	case state.canStart():
		if state.stage {
			return "Tiếp tục thêm hoặc nhấn Ctrl+S để áp dụng hướng và tiếp tục tạo"
		}
		return "Tiếp tục thêm hoặc nhấn Ctrl+S để bắt đầu tạo"
	default:
		return "Tiếp tục thêm yêu cầu của bạn, Enter để gửi cho AI"
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

type cocreateState struct {
	session    *startup.CoCreateSession
	stage      bool // true=đồng sáng tạo giai đoạn (lập kế hoạch hướng đi tiếp theo trong quá trình vận hành); false=đồng sáng tạo khởi đầu nguội (làm rõ các yêu cầu trước khi ra mắt)
	awaiting   bool
	reqID      int
	cancel     context.CancelFunc // Hủy yêu cầu LLM hiện tại
	deltaCh    chan cocreateStreamItem
	doneCh     chan cocreateDoneMsg
	convVP     viewport.Model
	promptVP   viewport.Model
	convFollow bool // true: Nội dung mới phát trực tuyến sẽ tự động cuộn xuống phía dưới; người dùng sẽ đặt sai để dừng theo dõi sau khi cuộn.
	// focusPrompt xác định cột nào sẽ cuộn trong ↑↓/PgUp/PgDn/Home/End: false=left cột hội thoại (mặc định),
	// true=thanh lệnh tạo bên phải. Trang chào mừng đã tắt tính năng báo cáo bằng chuột (giữ lại bản gốc) và tràn cột bên phải bằng cách nhấn Tab để chuyển tiêu điểm rồi cuộn bằng bàn phím.
	focusPrompt bool
}

func newCoCreateState(initial string) *cocreateState {
	makeVP := func() viewport.Model {
		vp := viewport.New(0, 0)
		vp.MouseWheelEnabled = true
		vp.MouseWheelDelta = 3
		return vp
	}
	return &cocreateState{
		session:    startup.NewCoCreateSession(strings.TrimSpace(initial)),
		awaiting:   true,
		convVP:     makeVP(),
		promptVP:   makeVP(),
		convFollow: true,
	}
}

// stageCoCreateOpener là cụm từ mở đầu tổng hợp của người dùng trong quá trình đồng sáng tạo giai đoạn, được gửi tới LLM khi người dùng bắt đầu.
// Hãy để trợ lý chủ động bắt đầu dựa trên “trạng thái câu chuyện hiện tại” thay vì đợi người dùng nói trước.
const stageCoCreateOpener = "Hãy để tôi tạm dừng một chút để lên kế hoạch cho bước đi tiếp theo với bạn."

// stageCoCreateSystemLine là cách trình bày trung lập về phần mở đầu này trong giao diện người dùng: câu mở đầu về cơ bản được hệ thống tổng hợp.
// Người dùng chưa thực sự gõ nên thay vì giả vờ là "bạn", dòng hệ thống được dùng để giải thích ngữ cảnh (nó vẫn sử dụng stageCoCreateOpener
// Đã gửi tới LLM, xem i==0 trường hợp đặc biệt của renderCoCreateConversationPanel).
const stageCoCreateSystemLine = "Quá trình sáng tạo đã bị tạm dừng, bước vào giai đoạn đồng sáng tạo - AI sẽ kết hợp diễn biến câu chuyện hiện tại và cùng bạn lên kế hoạch cho hướng đi tiếp theo."

// newStageCoCreateState tạo trạng thái đồng sáng tạo giai đoạn: hạt giống mở ra và đánh dấu giai đoạn để thực hiện runCoCreate
// StageCoCreateStream, Ctrl+S chuyển đến ResumeFromCoCreate.
func newStageCoCreateState() *cocreateState {
	s := newCoCreateState(stageCoCreateOpener)
	s.stage = true
	return s
}

func (s *cocreateState) appendUser(text string) {
	s.session.AppendUser(text)
}

func (s *cocreateState) apply(reply host.CoCreateReply) {
	s.awaiting = false
	s.session.ApplyReply(reply)
}

func (s *cocreateState) applyDelta(kind, text string) {
	s.session.ApplyDelta(kind, text)
}

func (s *cocreateState) canStart() bool {
	return s.session.CanStart()
}

func (s *cocreateState) initialInput() string {
	return s.session.InitialInput()
}

func (s *cocreateState) streamReply() string {
	return s.session.StreamReply()
}

func (s *cocreateState) draftPrompt() string {
	return s.session.DraftPrompt()
}

func (s *cocreateState) ready() bool {
	return s.session.Ready()
}

func (s *cocreateState) suggestions() []string {
	return s.session.Suggestions()
}

func (s *cocreateState) buildPlan() (startup.Plan, error) {
	return s.session.BuildPlan()
}

func renderStartupModeBar(width int, mode startupMode) string {
	quick := renderStartupModePill(mode == startupModeQuick, "bắt đầu nhanh")
	cocreate := renderStartupModePill(mode == startupModeCoCreate, "Đồng sáng tạo quy hoạch")
	title := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render("chế độ khởi động")
	divider := lipgloss.NewStyle().
		Foreground(colorDim).
		Render("·")
	line := title + " " + divider + " " + quick + "  " + cocreate
	return lipgloss.NewStyle().
		Width(width).
		Padding(0, 1).
		Render(line)
}

func renderStartupModePill(active bool, label string) string {
	style := lipgloss.NewStyle().Padding(0, 1)
	if active {
		style = style.Foreground(lipgloss.Color("#1c1a14")).Background(colorAccent).Bold(true)
	} else {
		style = style.Foreground(colorMuted)
	}
	return style.Render(label)
}

// coCreateColumns cắt vùng nội dung phương thức thành các cột bên trái và bên phải.
// Cột bên trái chứa các hộp thoại và hộp nhập liệu (xếp chồng lên nhau) và cột bên phải chứa các hướng dẫn sáng tạo nháp; tổng bằng với chiều rộng nội dung phương thức.
func coCreateColumns(bodyW int) (leftW, rightW int) {
	leftW = bodyW * 58 / 100
	if leftW < 42 {
		leftW = bodyW / 2
	}
	rightW = bodyW - leftW
	if rightW < 28 {
		rightW = 28
		leftW = bodyW - rightW
	}
	return leftW, rightW
}

func renderCoCreateBody(width, height int, state *cocreateState, errMsg, inputView string, spinnerFrame int) string {
	if state == nil {
		return ""
	}
	leftW, rightW := coCreateColumns(width)

	// Đường viền bên phải được vẽ bởi thùng chứa leftCol bên ngoài và chạy xuyên suốt phần thân từ trên xuống dưới; cuộc trò chuyện/gợi ý/
	// Đầu vào không vẽ đường viền bên phải của chính nó. Đầu vào vẫn là một ô tròn hoàn toàn, có 1 cột lề và 1 cột bên trái và bên phải.
	// Phần đệm của cuộc trò chuyện được căn chỉnh sao cho phù hợp với khoảng cách giữa hai bên.
	// Trong chế độ đồng sáng tạo, vùng văn bản được cố định thành 1 hàng (xem nhánh model.refitTextareaHeight),
	// chiều cao đầu vào = 1 (vùng văn bản) + 2 (viền trên/dưới) = 3 dòng, không bao giờ trôi.
	innerW := leftW - 1 // Để lại 1 cột cho đường dọc bên phải bên ngoài

	inputBox := lipgloss.NewStyle().
		Width(innerW-6). // -2 margin -2 padding -2 border
		Border(baseBorder).
		BorderForeground(colorDim).
		Padding(0, 1).
		Margin(0, 1).
		Render(inputView)

	suggestionsBox := renderCoCreateSuggestions(innerW, state)
	suggestionsH := 0
	if suggestionsBox != "" {
		suggestionsH = lipgloss.Height(suggestionsBox)
	}

	convH := height - lipgloss.Height(inputBox) - suggestionsH
	if convH < 4 {
		convH = 4
	}

	convPanel := renderCoCreateConversationPanel(innerW, convH, state, errMsg, spinnerFrame)

	var stack string
	if suggestionsBox == "" {
		stack = lipgloss.JoinVertical(lipgloss.Left, convPanel, inputBox)
	} else {
		stack = lipgloss.JoinVertical(lipgloss.Left, convPanel, suggestionsBox, inputBox)
	}

	leftCol := lipgloss.NewStyle().
		Border(baseBorder, false, true, false, false).
		BorderForeground(colorDim).
		Render(stack)

	rightPanel := renderCoCreatePromptPanel(rightW, height, state)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightPanel)
}

// extractReplyForDisplay trích xuất các phân đoạn <reply>...</reply> từ nội dung lịch sử trợ lý.
// Các thẻ khác (<draft>/<ready>/<suggestions>) là các trường giao thức cho vòng mô hình tiếp theo và không được hiển thị cho người dùng.
// Khi mô hình ở dạng bán tuân thủ (thiếu thẻ mở <reply>), câu trả lời sẽ bắt đầu bằng </reply> hoặc thẻ mở tiếp theo.
// Khi nó hoàn toàn không chứa bất kỳ thẻ nào (đường dẫn hạ cấp), nó sẽ được trả về nguyên trạng.
func extractReplyForDisplay(content string) string {
	rest := content
	if rIdx := strings.Index(content, "<reply>"); rIdx >= 0 {
		rest = content[rIdx+len("<reply>"):]
	}
	if cIdx := strings.Index(rest, "</reply>"); cIdx >= 0 {
		return strings.TrimSpace(rest[:cIdx])
	}
	cut := len(rest)
	for _, mark := range []string{"<draft>", "<ready>", "<suggestions>"} {
		if idx := strings.Index(rest, mark); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	if cut == len(rest) && !strings.Contains(content, "<") {
		return content
	}
	return strings.TrimSpace(rest[:cut])
}

// renderCoCreateSuggestions Hiển thị hàng đề xuất AI phía trên đầu vào. Khi chờ đợi hoặc khi không có gợi ý
// Trả về một chuỗi trống, cho phép bố cục tự động thu gọn mà không để lại dòng trống. Số lượng gợi ý tối đa là 3, nhấn các phím số 1/2/3 để chọn.
func renderCoCreateSuggestions(width int, state *cocreateState) string {
	if state == nil || state.awaiting {
		return ""
	}
	sugs := state.suggestions()
	if len(sugs) == 0 {
		return ""
	}
	if len(sugs) > 3 {
		sugs = sugs[:3]
	}

	digits := []string{"❶", "❷", "❸"}
	digitStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(colorMuted)
	hintStyle := lipgloss.NewStyle().Foreground(colorDim).Italic(true)

	lines := []string{hintStyle.Render("AI gợi ý (nhấn phím số để điền vào ô nhập):")}
	for i, s := range sugs {
		lines = append(lines, digitStyle.Render(digits[i]+" ")+bodyStyle.Render(strings.TrimSpace(s)))
	}

	// Căn chỉnh với lề/khoảng đệm bên trái và bên phải của inputBox: 2 cột ở bên trái (lề1+padding1), bên phải giống nhau.
	return lipgloss.NewStyle().
		Width(width-2).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))
}

func coCreateModalSize(width, height int) (boxW, boxH int) {
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 24
	}
	boxW = minInt(maxInt(width*76/100, 88), width-4)
	boxH = minInt(maxInt(height*72/100, 22), height-4)
	if boxW < 64 {
		boxW = maxInt(width-2, 42)
	}
	if boxH < 14 {
		boxH = maxInt(height-2, 12)
	}
	return boxW, boxH
}

// coCreateInputWidth tính toán độ rộng ký tự đầu vào thực tế của vùng văn bản.
// Trang trí cột bên trái: ngoài cùng bên phải đường dọc 1 + nhập lề trái và phải 2 + viền 2 + đệm 2 = 7 cột;
// chính vùng văn bản dấu nhắc+con trỏ chiếm 2 cột; vậy textareaW = leftW - 9.
func coCreateInputWidth(width, height int) int {
	boxW, _ := coCreateModalSize(width, height)
	bodyW := boxW - 4
	leftW, _ := coCreateColumns(bodyW)
	inputW := leftW - 9
	if inputW < 20 {
		inputW = 20
	}
	return inputW
}

func renderCoCreateModal(width, height int, state *cocreateState, errMsg, inputView string, spinnerFrame int, quitPending bool) string {
	if state == nil {
		return ""
	}

	boxW, boxH := coCreateModalSize(width, height)

	// Đặt tiêu đề/phụ đề/gợi ý bên ngoài phương thức (ở giữa bên trên và bên dưới) và đặt phương thức bên trong
	// Để nó hoàn toàn cho phần thân - cột bên trái, đường thẳng đứng bên phải và cột bên phải chạy từ trên cùng của phương thức xuống dưới cùng.
	// Nghề nghiệp thực tế theo phương thức = boxH (nội dung) + 2 (đệm 1*2) + 2 (viền) = boxH+4 dòng;
	// Ngăn xếp tổng thể = tiêu đề(1) + phụ đề(1) + trống(1) + modal(boxH+4) + trống(1) + gợi ý(1) = boxH+9.
	// Do đó, boxH giảm được 5 dòng ngân sách dành cho việc trang trí bên ngoài modal để tránh tràn thiết bị đầu cuối.
	contentH := boxH - 5
	if contentH < 10 {
		contentH = 10
	}

	titleText, subtitleText := "Đồng sáng tạo quy hoạch", "Nói rõ ràng về nhu cầu của bạn trước khi bắt đầu tạo"
	if state.stage {
		titleText, subtitleText = "đồng sáng tạo sân khấu", "Lập kế hoạch hướng đi tiếp theo trước khi tiếp tục sáng tạo"
	}
	headerStyle := lipgloss.NewStyle().Width(boxW).AlignHorizontal(lipgloss.Center)
	title := headerStyle.Foreground(colorMuted).Bold(true).Render(titleText)
	subtitle := headerStyle.Foreground(colorDim).Italic(true).Render(subtitleText)

	var hintLine string
	hintStyle := lipgloss.NewStyle().Width(boxW).AlignHorizontal(lipgloss.Center)
	if quitPending {
		// quitPending nhất quán với inputHints(); nếu không thì phương thức đồng tạo sẽ bao phủ thanh dưới cùng và người dùng không thể cảm thấy "nhấn Ctrl+C lần nữa".
		hintLine = hintStyle.Foreground(lipgloss.Color("243")).Bold(true).Render("Press Ctrl+C again to exit")
	} else {
		hintLine = hintStyle.Foreground(colorDim).Italic(true).Render(coCreateHint(state))
	}

	body := renderCoCreateBody(boxW-4, contentH, state, errMsg, inputView, spinnerFrame)
	box := lipgloss.NewStyle().
		Width(boxW).
		Height(contentH).
		Border(baseBorder).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Render(body)

	stack := lipgloss.JoinVertical(lipgloss.Center, title, subtitle, "", box, "", hintLine)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, stack)
}

// coCreateHint tạo các gợi ý chính ngắn dựa trên trạng thái để tránh trùng lặp ngữ nghĩa với phần giữ chỗ.
func coCreateHint(state *cocreateState) string {
	switch {
	case state == nil:
		return "Enter để gửi · Esc để thoát"
	case state.awaiting:
		return "AI trả lời · ↑↓ Cuộc trò chuyện cuộn · Lệnh cuộn bằng bánh xe · Esc để thoát"
	case state.canStart():
		action := "Ctrl+S bắt đầu tạo"
		if state.stage {
			action = "Ctrl+S Áp dụng và tiếp tục"
		}
		return "Nhập để tiếp tục thêm · " + action + " · ↑↓ hội thoại cuộn · lệnh cuộn bánh xe · Esc để thoát"
	default:
		return "Enter để gửi · ↑↓ Đoạn hội thoại cuộn · Lệnh cuộn bánh xe · Esc để thoát"
	}
}

func renderCoCreateConversationPanel(width, height int, state *cocreateState, errMsg string, spinnerFrame int) string {
	// Không vẽ đường viền riêng - đường thẳng đứng bên phải được vẽ đồng đều bởi vùng chứa leftCol bên ngoài.
	// Tổng chiều rộng cột = chiều rộng; style.Width = contentW = width-2; vùng nội dung sau Padding(0,1) = contentW-2.
	// Tiền tố 2 cột như "▌ " / " " cũng phải được trừ trong hàng, nếu không tiền tố + của mỗi hàng sau khi gói sẽ tràn 2 cột của vùng nội dung.
	// Kích hoạt gói vật lý của thiết bị đầu cuối - lipgloss vẫn coi chiều cao phương thức là cố định, nhưng chiều cao hiển thị thực tế của thiết bị đầu cuối tăng lên.
	// Nếu nó tiếp tục kích hoạt trong quá trình suy nghĩ trực tuyến, nó sẽ xuất hiện dưới dạng "độ giật cao" ở khung bên ngoài. Vậy bọcW = nội dungW - 4.
	contentW := width - 2
	if contentW < 12 {
		contentW = 12
	}
	wrapW := max(12, contentW-4)

	userRole := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render("Bạn")
	aiRole := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("AI")
	userBody := lipgloss.NewStyle().Foreground(colorAccent2)
	aiBody := lipgloss.NewStyle().Foreground(bodyTextColor)
	thinkingStyle := lipgloss.NewStyle().Foreground(colorDim).Italic(true)
	thinkingTag := lipgloss.NewStyle().Foreground(colorDim).Bold(true).Render("Tư duy AI")

	sysStyle := lipgloss.NewStyle().Foreground(colorDim).Italic(true)

	var lines []string
	for i, item := range state.session.History() {
		isUser := item.Role != "assistant"
		// Phần mở đầu của phần đồng sáng tạo sân khấu (luôn là thông báo lịch sử của người dùng[0]) được hiển thị bằng một dòng hệ thống trung tính,
		// Không được ngụy trang dưới dạng đầu vào của người dùng; nó vẫn được gửi đến LLM khi lượt người dùng bắt đầu.
		if isUser && state.stage && i == 0 {
			for j, line := range wrapStreamText(stageCoCreateSystemLine, wrapW) {
				prefix := "· "
				if j > 0 {
					prefix = "  "
				}
				lines = append(lines, sysStyle.Render(prefix+line))
			}
			lines = append(lines, "")
			continue
		}
		if isUser {
			lines = append(lines, userRole)
			for _, line := range wrapStreamText(strings.TrimSpace(item.Content), wrapW) {
				// Hiển thị toàn bộ dòng cùng một lúc để tránh ký tự điều khiển ANSI bị tràn trong đó màu đặt lại tiền tố và màu văn bản được ghép nối.
				lines = append(lines, userBody.Render("▌ "+line))
			}
		} else {
			lines = append(lines, aiRole)
			// Trong lịch sử, trợ lý lưu trữ bốn phân đoạn hoàn chỉnh của Nguyên (được sử dụng cho ngữ cảnh mô hình) và giao diện người dùng chỉ hiển thị phân đoạn [TRẢ LỜI].
			display := extractReplyForDisplay(item.Content)
			for _, line := range wrapStreamText(strings.TrimSpace(display), wrapW) {
				lines = append(lines, aiBody.Render("  "+line))
			}
		}
		lines = append(lines, "")
	}

	if state.awaiting {
		if t := state.session.StreamThinking(); t != "" {
			lines = append(lines, thinkingTag)
			for _, line := range wrapStreamText(t, wrapW) {
				lines = append(lines, thinkingStyle.Render("  "+line))
			}
			lines = append(lines, "")
		}
		if state.streamReply() != "" {
			lines = append(lines, aiRole)
			for _, line := range wrapStreamText(state.streamReply(), wrapW) {
				lines = append(lines, aiBody.Render("  "+line))
			}
			lines = append(lines, "")
		}
		// trang trí lấp lánh: để người dùng luôn thấy "AI tại nơi làm việc"
		lines = append(lines, strings.TrimLeft(renderEventSparkle(spinnerFrame, contentW), " "))
	}
	if errMsg != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(colorError).Render("! "+errMsg))
	}

	// Sử dụng chế độ xem thay vì cắt bớt thủ công để cho phép người dùng cuộn lại.
	// chiều cao vp = chiều cao bảng - tiêu đề 1 hàng. Sau SetContent, nếu người dùng ban đầu ở dưới cùng,
	// Tự động cuộn đến mới nhất (phát trực tuyến sau); sau khi người dùng cuộn, convFollow sẽ bị tắt và thao tác sau sẽ dừng.
	vpH := height - 1
	if vpH < 1 {
		vpH = 1
	}
	if state.convVP.Width != contentW || state.convVP.Height != vpH {
		state.convVP.Width = contentW
		state.convVP.Height = vpH
	}
	state.convVP.SetContent(strings.Join(lines, "\n"))
	if state.convFollow {
		state.convVP.GotoBottom()
	}

	style := lipgloss.NewStyle().
		Width(contentW).
		Height(height).
		Padding(0, 1)
	return style.Render(panelTitleStyle.Render(":: Đồng tạo cuộc trò chuyện") + "\n" + state.convVP.View())
}

func renderCoCreatePromptPanel(width, height int, state *cocreateState) string {
	readyLabel := "Sẵn sàng để bắt đầu tạo"
	if state.stage {
		readyLabel = "Sẵn sàng đăng ký và tiếp tục"
	}
	status := lipgloss.NewStyle().Foreground(colorDim).Render("Cuộc trò chuyện tiếp tục")
	if state.ready() {
		status = lipgloss.NewStyle().Foreground(colorAccent).Render(readyLabel)
	}
	if state.awaiting {
		status = lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render("AI đang phân loại")
	}

	// Chiều rộng nội dung = tổng chiều rộng cột - 2 (phần đệm 0,1 chiếm 2 cột, không có viền).
	contentW := width - 2
	if contentW < 8 {
		contentW = 8
	}

	emptyHint := "AI sẽ tiếp tục sắp xếp hướng dẫn cuối cùng ở đây để có thể trực tiếp đưa vào quá trình sáng tạo."
	panelTitle := "::Lệnh tạo hiện tại"
	if state.stage {
		emptyHint = "AI sẽ tiếp tục soạn thảo bản tóm tắt định hướng cho các giai đoạn tiếp theo tại đây."
		panelTitle = "::Làm theo hướng dẫn"
	}
	text := strings.TrimSpace(state.draftPrompt())
	if text == "" {
		text = lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render(emptyHint)
	} else {
		text = renderMarkdownPreview(text, max(12, contentW-2))
	}
	vpHeight := height - 5
	if vpHeight < 3 {
		vpHeight = 3
	}
	if state.promptVP.Width != contentW || state.promptVP.Height != vpHeight {
		state.promptVP.Width = contentW
		state.promptVP.Height = vpHeight
	}
	state.promptVP.MouseWheelEnabled = true
	state.promptVP.SetContent(text)

	hint := ""
	if state.promptVP.TotalLineCount() > state.promptVP.VisibleLineCount() {
		switch {
		case state.promptVP.AtTop():
			hint = "↓ Bên dưới còn nhiều nội dung khác, các bạn có thể cuộn hoặc PgDn để xem"
		case state.promptVP.AtBottom():
			hint = "↑ Còn nhiều nội dung phía trên, có thể xem bằng con lăn hoặc PGUp"
		default:
			hint = "↑↓ Tiếp tục cuộn để xem"
		}
	}

	style := lipgloss.NewStyle().
		Width(contentW).
		Height(height).
		Padding(0, 1)

	body := panelTitleStyle.Render(panelTitle) + "\n" + status + "\n\n" + state.promptVP.View()
	if hint != "" {
		body += "\n\n" + lipgloss.NewStyle().
			Width(contentW).
			AlignHorizontal(lipgloss.Center).
			Foreground(colorDim).
			Italic(true).
			Render(hint)
	}
	return style.Render(body)
}

func renderMarkdownPreview(text string, width int) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}

	h1Style := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	h2Style := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
	h3Style := lipgloss.NewStyle().Foreground(colorMuted).Bold(true)
	bulletStyle := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
	codeStyle := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	var out []string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			out = append(out, "")
			continue
		}

		switch {
		case strings.HasPrefix(line, "# "):
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			out = append(out, h1Style.Render(title))
		case strings.HasPrefix(line, "## "):
			title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			out = append(out, h2Style.Render(title))
		case strings.HasPrefix(line, "### "):
			title := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			out = append(out, h3Style.Render(title))
		case strings.HasPrefix(line, "- "), strings.HasPrefix(line, "* "):
			body := strings.TrimSpace(line[2:])
			wrapped := wrapStreamText(body, max(8, width-4))
			for i, item := range wrapped {
				if i == 0 {
					out = append(out, bulletStyle.Render("• ")+cardContentStyle.Render(item))
				} else {
					out = append(out, "  "+cardContentStyle.Render(item))
				}
			}
		case isOrderedMarkdownItem(line):
			prefix, body := splitOrderedMarkdownItem(line)
			wrapped := wrapStreamText(body, max(8, width-len(prefix)-2))
			for i, item := range wrapped {
				if i == 0 {
					out = append(out, bulletStyle.Render(prefix+" ")+cardContentStyle.Render(item))
				} else {
					out = append(out, strings.Repeat(" ", len(prefix)+1)+cardContentStyle.Render(item))
				}
			}
		case strings.HasPrefix(line, "> "):
			body := strings.TrimSpace(strings.TrimPrefix(line, "> "))
			for _, item := range wrapStreamText(body, max(8, width-4)) {
				out = append(out, codeStyle.Render("│ "+item))
			}
		default:
			for _, item := range wrapStreamText(line, width) {
				out = append(out, cardContentStyle.Render(item))
			}
		}
	}
	return strings.Join(out, "\n")
}

func isOrderedMarkdownItem(line string) bool {
	if len(line) < 3 {
		return false
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' '
}

func splitOrderedMarkdownItem(line string) (prefix, body string) {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) {
		return "", strings.TrimSpace(line)
	}
	return line[:i+1], strings.TrimSpace(line[i+2:])
}
