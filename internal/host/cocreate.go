package host

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/bootstrap"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// Đồng sáng tạo bắt đầu từ đầu: làm rõ các yêu cầu ngay từ đầu và đưa ra hướng dẫn sáng tạo cho toàn bộ cuốn sách.
const coCreateSystemPrompt = `Bạn là một trợ lý đồng sáng tạo mới lạ. Nhiệm vụ của bạn không phải là bắt đầu viết tiểu thuyết trực tiếp mà là giúp người dùng làm rõ nhu cầu sáng tạo của họ thông qua nhiều vòng trò chuyện ngắn và tiếp tục sắp xếp hướng dẫn sáng tạo bằng tiếng Trung để có thể chuyển trực tiếp cho công cụ sáng tạo.

Mỗi vòng phản hồi được xuất ra theo đúng định dạng XML sau, chứa bốn thẻ, xuất hiện theo trình tự. Mỗi thẻ phải có thẻ mở và thẻ đóng chính xác:

<trả lời>
Trả lời tự nhiên bằng tiếng Trung cho người dùng: trước tiên hãy trả lời ý kiến ​​đóng góp của người dùng, sau đó hỏi tối đa 1 đến 2 câu hỏi quan trọng nhất vào lúc này. Nếu có đủ thông tin để bắt đầu soạn thảo, hãy nói với người dùng rằng họ có thể nhấn Ctrl+S để bắt đầu.
</reply>

<dự thảo>
Bản thảo hướng dẫn soạn thảo hoàn chỉnh hiện tại, sử dụng Markdown: bắt đầu trực tiếp với các tiêu đề phụ, chẳng hạn như "## Chủ đề", "## Các yếu tố chính", "## Thông tin cần làm rõ"; sử dụng dấu đầu dòng để liệt kê những điểm chính. Mỗi vòng phải **cập nhật tích lũy** các kết luận hiện có và tiếp thu ý định mới nhất của người dùng; ngay cả khi không có bổ sung mới trong vòng này, bản nháp hoàn chỉnh vẫn phải được viết lại như cũ - không bỏ sót hoặc ghi các phần giữ chỗ như "(Giữ vòng trước)".
</dự thảo>
` + coCreateProtocolTail

// Đồng sáng tạo theo giai đoạn: Một phần của cuốn tiểu thuyết đã được viết và định hướng của "các giai đoạn tiếp theo" đã được lên kế hoạch. Người gọi cần cung cấp bản tóm tắt về trạng thái câu chuyện hiện tại
// Thêm vào sau lời nhắc này đoạn ("## Trạng thái câu chuyện hiện tại") để mô hình lập kế hoạch dựa trên những gì đã được viết.
const stageCoCreateSystemPrompt = `Bạn là một trợ lý "đồng sáng tạo sân khấu" mới lạ. Cuốn tiểu thuyết này đã được viết một phần (xem "Trạng thái câu chuyện hiện tại" bên dưới để biết tiến trình). Người dùng tạm dừng và muốn cùng bạn lên kế hoạch hướng đi cho “giai đoạn tiếp theo” trước khi tiếp tục sáng tạo.

Nhiệm vụ của bạn không phải là tiếp tục viết văn bản chính mà là giúp người dùng tìm ra phần tiếp theo (vài chương tiếp theo/phần tiếp theo/tập tiếp theo) thông qua nhiều vòng đối thoại ngắn và tiếp tục sắp xếp một "tóm tắt hướng tiếp theo" để công cụ sáng tạo phát triển tương ứng.

Nguyên tắc sắt: Mọi gợi ý phải phù hợp với cốt truyện, nhân vật, điềm báo đã xảy ra trong “trạng thái câu chuyện hiện tại”. Đừng bao giờ lật đổ hoặc bỏ qua những gì đã được viết ra; chỉ lập kế hoạch “làm thế nào để tiến hành” và không thiết kế lại toàn bộ cuốn sách.

Mỗi vòng phản hồi được xuất ra theo đúng định dạng XML sau, chứa bốn thẻ, xuất hiện theo trình tự. Mỗi thẻ phải có thẻ mở và thẻ đóng chính xác:

<trả lời>
Trả lời tự nhiên bằng tiếng Trung cho người dùng: trước tiên hãy trả lời ý kiến ​​đóng góp của người dùng, sau đó hỏi tối đa 1 đến 2 câu hỏi quan trọng nhất vào lúc này. Nếu hướng tiếp theo đủ rõ ràng, hãy cho người dùng biết rằng họ có thể nhấn Ctrl+S để chuyển hướng cho công cụ sáng tạo và tiếp tục tạo.
</reply>

<dự thảo>
"Bản tóm tắt hướng tiếp theo" hoàn chỉnh hiện tại, sử dụng Markdown: bắt đầu trực tiếp từ các tiêu đề cấp hai, chẳng hạn như "## Hướng tiếp theo", "## Xoay phím", "## Điềm báo sẽ được thực hiện", "## Nhịp điệu và độ dài"; sử dụng dấu đầu dòng để liệt kê những điểm chính. Mỗi vòng phải **cập nhật tích lũy** các kết luận hiện có để tiếp thu ý định mới nhất của người dùng; ngay cả khi không có bổ sung mới trong vòng này, bản tóm tắt đầy đủ vẫn phải được viết lại như cũ - không bỏ sót hoặc viết các phần giữ chỗ như "(Giữ vòng trước)".
</dự thảo>
` + coCreateProtocolTail

// coCreateProtocolTail là đuôi giao thức đầu ra (<ready> / <suggestions> + thông số đầu ra) được chia sẻ bởi cả hai chế độ đồng sáng tạo.
// Hai chế độ này chỉ khác nhau về ngữ cảnh mở và <dự thảo>, đồng thời các giao thức hoàn toàn nhất quán.
const coCreateProtocolTail = `
<ready>sai</ready>

<gợi ý>
1-3 "Điều người dùng có thể muốn nói tiếp theo", mỗi dòng bắt đầu bằng "-". Đây là hướng dẫn cho người dùng khi họ gặp khó khăn.
Nhấn các phím số để điền vào ô nhập, người dùng có thể chỉnh sửa và gửi.

Yêu cầu:
- Nói như người dùng và nói những gì người dùng nói với bạn. Đừng viết nó như một câu hỏi tu từ của trợ lý.
- Mỗi bài viết không quá 25 từ, mẫu câu đa dạng, tránh giống nhau.
- Đưa ra xu hướng/lựa chọn/ý định bổ sung, không viết đầy đủ cài đặt cho người dùng trong một câu.
</gợi ý>

Thông số đầu ra:
- Phải sử dụng bốn thẻ XML: <reply> / <draft> / <ready> / <suggestions>, mỗi thẻ phải được mở và đóng hoàn toàn.
- Tên thẻ chỉ được viết bằng tiếng Anh viết thường và không được viết lại thành bất kỳ biến thể nào như <REPLY>/<REWRITE>/<reply>.
- Không thêm bất kỳ lời giải thích, phản ánh hoặc hàng rào mã nào bên ngoài thẻ.
- Cho phép nhiều dòng Markdown trong <draft>, có thể được viết trực tiếp bằng dấu ngắt dòng mà không cần thoát.
- <ready> chỉ viết đúng hoặc sai. Điền đúng khi thông tin đã đủ.
- <suggestions> có thể để trống khi <ready>true</ready> (chỉ cần giữ thẻ trống <suggestions></suggestions>).`

// CoCreateProgressKind Xác định loại nội dung của lệnh gọi lại phát trực tuyến.
const (
	CoCreateProgressThinking = "thinking"
	CoCreateProgressReply    = "reply"
)

// Đầu ra thẻ XML gồm bốn phần. Kiểu XML mạnh mẽ hơn dấu ngoặc vuông - Dữ liệu đào tạo Claude/GPT
// Đối với số lượng lớn các định dạng <thinking>...</thinking>, mô hình gần như sẽ không bao giờ viết lại <reply> thành <REWRITE>
// hoặc các biến thể khác; thẻ đóng cũng cho phép cắt ngắn giữa luồng chính xác hơn (không cần dựa vào việc tìm điểm đánh dấu tiếp theo để cắt đuôi).
const (
	tagReply       = "reply"
	tagDraft       = "draft"
	tagReady       = "ready"
	tagSuggestions = "suggestions"
)

func coCreateStream(ctx context.Context, models *bootstrap.ModelSet, sessions *store.SessionStore, sysPrompt string, history []CoCreateMessage, onProgress func(kind, text string)) (reply CoCreateReply, err error) {
	if len(history) == 0 {
		return CoCreateReply{}, fmt.Errorf("cocreate history is empty")
	}

	model := models.ForRole("thinking")
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	msgs := []agentcore.Message{agentcore.SystemMsg(sysPrompt)}
	for _, item := range history {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.Role)) {
		case "assistant":
			msgs = append(msgs, assistantMsg(content))
		default:
			msgs = append(msgs, agentcore.UserMsg(content))
		}
	}

	var raw, thinking strings.Builder

	// Việc khắc phục sự cố không thường xuyên như "đồng tạo phản hồi trống" yêu cầu xem mô hình thực sự trả về kết quả gì.
	// Toàn bộ quá trình của mỗi vòng được đặt trong <output>/meta/sessions/cocreate.jsonl, ở cùng vị trí với nhật ký phiên được tạo chính thức.
	start := time.Now()
	defer func() {
		if sessions == nil {
			return
		}
		_ = sessions.LogCoCreate(coCreateLogEntry{
			Time:         time.Now(),
			DurationMS:   time.Since(start).Milliseconds(),
			InputHistory: history,
			RawResponse:  raw.String(),
			RawLen:       len([]rune(raw.String())),
			Thinking:     thinking.String(),
			ParsedReply:  reply.Message,
			ParsedDraft:  reply.Prompt,
			ParsedReady:  reply.Ready,
			ParsedSugs:   reply.Suggestions,
			Error:        errString(err),
		})
	}()

	streamCh, err := model.GenerateStream(ctx, msgs, nil, agentcore.WithMaxTokens(2048))
	if err != nil {
		return CoCreateReply{}, fmt.Errorf("cocreate generate: %w", err)
	}

	var streamed bool
	for ev := range streamCh {
		switch ev.Type {
		case agentcore.StreamEventThinkingDelta:
			thinking.WriteString(ev.Delta)
			if onProgress != nil {
				onProgress(CoCreateProgressThinking, thinking.String())
			}
		case agentcore.StreamEventTextDelta:
			streamed = true
			raw.WriteString(ev.Delta)
			if onProgress != nil {
				onProgress(CoCreateProgressReply, extractReplyPreview(raw.String()))
			}
		case agentcore.StreamEventDone:
			if !streamed {
				raw.WriteString(ev.Message.TextContent())
			}
		case agentcore.StreamEventError:
			if ev.Err != nil {
				return CoCreateReply{}, fmt.Errorf("cocreate generate: %w", ev.Err)
			}
			return CoCreateReply{}, fmt.Errorf("cocreate generate failed")
		}
	}

	// Dự phòng kênh: Các mô hình tư duy (R1/GLM-Z1/QwQ, v.v.) thỉnh thoảng viết câu trả lời hoàn chỉnh vào
	// Reason_content không được chuyển trở lại kênh câu trả lời cuối cùng, dẫn đến raw bị trống nhưng suy nghĩ chứa đựng
	// Bốn đoạn văn hoàn chỉnh. Để đo lường thực tế, hãy xem meta/sessions/cocreate.jsonl - trực tiếp sử dụng tư duy làm nguyên liệu để phân tích.
	// Lớp giao thức đã bị hạ cấp (nếu không có dấu [REPLY] thì toàn bộ đoạn văn sẽ được sử dụng làm câu trả lời) và không có sự khác biệt nào về trải nghiệm giao diện người dùng sau khi giải cứu.
	rawText := raw.String()
	if strings.TrimSpace(rawText) == "" {
		if t := strings.TrimSpace(thinking.String()); t != "" {
			rawText = t
		}
	}
	reply, err = parseCoCreateResponse(rawText)
	return reply, err
}

// coCreateLogEntry là cấu trúc một dòng được viết vào meta/sessions/cocreate.jsonl.
// Việc đặt tên trường gần giống với thói quen truy vấn trực tiếp jsonl (snake_case), tạo điều kiện thuận lợi cho việc lọc jq.
type coCreateLogEntry struct {
	Time         time.Time         `json:"time"`
	DurationMS   int64             `json:"duration_ms"`
	InputHistory []CoCreateMessage `json:"input_history"`
	RawResponse  string            `json:"raw_response"`
	RawLen       int               `json:"raw_len"`
	Thinking     string            `json:"thinking,omitempty"`
	ParsedReply  string            `json:"parsed_reply"`
	ParsedDraft  string            `json:"parsed_draft"`
	ParsedReady  bool              `json:"parsed_ready"`
	ParsedSugs   []string          `json:"parsed_sugs,omitempty"`
	Error        string            `json:"error,omitempty"`
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func assistantMsg(text string) agentcore.Message {
	return agentcore.Message{
		Role:      agentcore.RoleAssistant,
		Content:   []agentcore.ContentBlock{agentcore.TextBlock(text)},
		Timestamp: time.Now(),
	}
}

// phân tích cú phápCoCreateResponse Phân tích đầu ra thẻ XML. Nếu mô hình không tuân thủ giao thức (nói trực tiếp bằng ngôn ngữ tự nhiên),
// Toàn bộ đoạn văn được hiển thị dưới dạng câu trả lời và bản nháp được để trống để phiên giữ lại vòng trước.
func parseCoCreateResponse(raw string) (CoCreateReply, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return CoCreateReply{}, fmt.Errorf("cocreate empty response")
	}

	reply, draft, ready, suggestions := splitCoCreateMarkers(raw)
	if reply == "" {
		// Mô hình không tuân thủ giao thức XML: toàn bộ đoạn văn được sử dụng làm câu trả lời.
		return CoCreateReply{Message: raw, Prompt: "", Ready: false, Raw: raw}, nil
	}
	return CoCreateReply{
		Message:     reply,
		Prompt:      draft,
		Ready:       ready,
		Suggestions: suggestions,
		Raw:         raw,
	}, nil
}

// SplitCoCreateMarkers chia văn bản thành bốn thẻ XML.
// Nhãn có thể bị thiếu (thiếu ở giữa luồng hoặc trong mô hình) và trường tương ứng cho phần bị thiếu là trống/false/nil.
// Khi thiếu thẻ đóng, extractTagContent sẽ được truy xuất đến cuối chuỗi và vẫn cố gắng hết sức để phân tích cú pháp.
func splitCoCreateMarkers(s string) (reply, draft string, ready bool, suggestions []string) {
	reply = extractTagContent(s, tagReply)
	draft = extractTagContent(s, tagDraft)
	readyStr := strings.ToLower(extractTagContent(s, tagReady))
	ready = readyStr == "true" || readyStr == "yes"
	suggestions = parseSuggestions(extractTagContent(s, tagSuggestions))
	return
}

// extractTagContent trích xuất văn bản giữa <tag>...</tag> từ s.
// Ba tình huống lỗi vô tình được đề cập để tránh trực tiếp hạ cấp và mất trường:
//  1. Mở hoặc đóng (giữa dòng) → chuyển sang thẻ mở đã biết tiếp theo
//  2. Không mở và đóng (lỗi đánh máy mẫu, chẳng hạn như <suggestions> được viết dưới dạng <uggestions>) → từ lần biết cuối cùng
//     Bắt đầu từ vị trí cuối của thẻ đóng hoàn chỉnh, trước </tag>
//  3. Trả lời hoàn toàn không có thẻ mở (mô hình bắt đầu trực tiếp bằng ngôn ngữ tự nhiên và dán </reply> ở cuối) → từ đầu đến </reply>
func extractTagContent(s, tag string) string {
	open := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	oIdx := strings.Index(s, open)
	if oIdx >= 0 {
		rest := s[oIdx+len(open):]
		if cIdx := strings.Index(rest, closeTag); cIdx >= 0 {
			return strings.TrimSpace(rest[:cIdx])
		}
		// Mở hoặc đóng → chuyển sang thẻ mở đã biết tiếp theo
		for _, other := range []string{"<reply>", "<draft>", "<ready>", "<suggestions>"} {
			if other == open {
				continue
			}
			if idx := strings.Index(rest, other); idx >= 0 {
				rest = rest[:idx]
			}
		}
		return strings.TrimSpace(rest)
	}

	// Không mở và đóng → Bắt đầu ở cuối thẻ đóng hoàn toàn đã biết cuối cùng, tới </tag>.
	if cIdx := strings.Index(s, closeTag); cIdx >= 0 {
		prefix := s[:cIdx]
		start := 0
		for _, t := range []string{"</reply>", "</draft>", "</ready>", "</suggestions>"} {
			if t == closeTag {
				continue
			}
			if i := strings.LastIndex(prefix, t); i >= 0 {
				if end := i + len(t); end > start {
					start = end
				}
			}
		}
		return strings.TrimSpace(prefix[start:])
	}
	return ""
}

// parsSuggestions Trích xuất từng dòng của đoạn <suggestions> và loại bỏ các tiền tố danh sách như "- " / "* " / "1. ".
// Giữ tối đa 3 mục; dòng trống, quá ngắn (<2 từ), toàn dòng như thẻ XML (thẻ mở lỗi đánh máy được để ở cuối,
// ví dụ. <gợi ý>) đều bị bỏ qua.
func parseSuggestions(text string) []string {
	if text == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Toàn bộ dòng giống như một thẻ XML → bỏ qua (để tránh ô nhiễm thẻ lỗi chính tả)
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			continue
		}
		// Tiền tố danh sách dải
		switch {
		case strings.HasPrefix(line, "- "):
			line = strings.TrimSpace(line[2:])
		case strings.HasPrefix(line, "* "):
			line = strings.TrimSpace(line[2:])
		case isOrderedSuggestion(line):
			line = stripOrderedPrefix(line)
		}
		if len([]rune(line)) < 2 {
			continue
		}
		out = append(out, line)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

// isOrderedSuggestion xác định xem phần đầu của dòng có dạng "1. " / "12. " (số + dấu chấm + dấu cách).
func isOrderedSuggestion(line string) bool {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' '
}

func stripOrderedPrefix(line string) string {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) {
		return line
	}
	return strings.TrimSpace(line[i+2:])
}

// extractReplyPreview xem trước trực tuyến: cung cấp cho giao diện người dùng một văn bản có thể hiển thị trong khi bản thô vẫn đang phát triển.
// Tìm nội dung sau <reply> và chuyển sang </reply> hoặc trước thẻ mở tiếp theo <draft>.
// Khi mô hình ở dạng bán tuân thủ (thiếu thẻ <reply>), mọi thứ từ đầu đến </reply> hoặc <draft> đều được coi là phản hồi.
func extractReplyPreview(raw string) string {
	trimmed := strings.TrimSpace(raw)
	open := "<" + tagReply + ">"
	closeTag := "</" + tagReply + ">"
	draftOpen := "<" + tagDraft + ">"

	rest := trimmed
	if rIdx := strings.Index(trimmed, open); rIdx >= 0 {
		rest = trimmed[rIdx+len(open):]
	}
	if cIdx := strings.Index(rest, closeTag); cIdx >= 0 {
		return strings.TrimSpace(rest[:cIdx])
	}
	if dIdx := strings.Index(rest, draftOpen); dIdx >= 0 {
		rest = rest[:dIdx]
	}
	return strings.TrimSpace(rest)
}
