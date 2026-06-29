package startup

import (
	"fmt"
	"strings"

	"github.com/nguyenducvuongg/ainovelViet/internal/host"
)

// CoCreateSession mang trạng thái không phải UI cho chế độ đồng sáng tạo.
type CoCreateSession struct {
	history        []host.CoCreateMessage
	draftPrompt    string
	ready          bool
	streamReply    string
	streamThinking string
	suggestions    []string
}

func NewCoCreateSession(initial string) *CoCreateSession {
	return &CoCreateSession{
		history: []host.CoCreateMessage{
			{Role: "user", Content: strings.TrimSpace(initial)},
		},
	}
}

func (s *CoCreateSession) History() []host.CoCreateMessage {
	if s == nil {
		return nil
	}
	return append([]host.CoCreateMessage(nil), s.history...)
}

func (s *CoCreateSession) ApplyReply(reply host.CoCreateReply) {
	if s == nil {
		return
	}
	s.streamReply = ""
	s.streamThinking = ""
	// Trong lịch sử, trợ lý lưu ba phần hoàn chỉnh của Raw (bao gồm [DRAFT]), chỉ có thể nhìn thấy trong vòng mô hình tiếp theo.
	// Bản nháp tôi đã viết ở vòng trước và các bản cập nhật tích lũy dựa trên nó; chỉ lưu Tin nhắn sẽ tạo [DRAFT] hoàn toàn
	// Nếu không có ngữ cảnh, mô hình chỉ có thể được tóm tắt lại dựa trên đoạn hội thoại trong mỗi vòng và các chi tiết ban đầu rất dễ bị bỏ qua. Đường dẫn hạ cấp
	// Nguyên == Tin nhắn, tương đương.
	text := strings.TrimSpace(reply.Raw)
	if text == "" {
		text = strings.TrimSpace(reply.Message)
	}
	if text != "" {
		s.history = append(s.history, host.CoCreateMessage{Role: "assistant", Content: text})
	}
	// Chỉ ghi đè bản nháp nếu Lời nhắc không trống: đường dẫn hạ cấp phân tích cú pháp sẽ trả về Lời nhắc = "", trong trường hợp đó
	// Vòng dự thảo trước đó phải được giữ lại, nếu không, "hướng dẫn sáng tạo hiện tại" do người dùng tích lũy sẽ bị xóa bằng câu trả lời bị cắt ngắn.
	if prompt := strings.TrimSpace(reply.Prompt); prompt != "" {
		s.draftPrompt = prompt
	}
	s.ready = reply.Ready
	// Đề xuất đưa tin trực tiếp (bao gồm cả thông tin trống): mỗi vòng hướng dẫn chỉ có ý nghĩa đối với thời điểm hiện tại.
	s.suggestions = append(s.suggestions[:0], reply.Suggestions...)
}

func (s *CoCreateSession) AppendUser(text string) {
	if s == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// Người dùng đã quyết định phải nói gì tiếp theo và các đề xuất ngay lập tức bị vô hiệu hóa để tránh trường hợp AI chưa phản hồi.
	// Đề xuất cũ treo trên hộp nhập liệu đã gây nhầm lẫn.
	s.suggestions = nil
	s.history = append(s.history, host.CoCreateMessage{Role: "user", Content: text})
}

// ApplyDelta nhận được tích lũy phát trực tuyến; kind="thinking" ghi luồng suy luận và "reply" ghi bản xem trước trả lời.
// Hai kênh được tích lũy riêng biệt và giao diện người dùng có thể được tô màu và hiển thị theo khối, cho phép người dùng xem LLM hoạt động trong giai đoạn suy nghĩ.
func (s *CoCreateSession) ApplyDelta(kind, text string) {
	if s == nil {
		return
	}
	text = strings.TrimSpace(text)
	switch kind {
	case host.CoCreateProgressThinking:
		s.streamThinking = text
	case host.CoCreateProgressReply:
		s.streamReply = text
	}
}

func (s *CoCreateSession) StreamReply() string {
	if s == nil {
		return ""
	}
	return s.streamReply
}

func (s *CoCreateSession) StreamThinking() string {
	if s == nil {
		return ""
	}
	return s.streamThinking
}

func (s *CoCreateSession) DraftPrompt() string {
	if s == nil {
		return ""
	}
	return s.draftPrompt
}

func (s *CoCreateSession) Suggestions() []string {
	if s == nil {
		return nil
	}
	return s.suggestions
}

func (s *CoCreateSession) Ready() bool {
	if s == nil {
		return false
	}
	return s.ready
}

func (s *CoCreateSession) CanStart() bool {
	return strings.TrimSpace(s.DraftPrompt()) != ""
}

func (s *CoCreateSession) InitialInput() string {
	if s == nil || len(s.history) == 0 {
		return ""
	}
	return strings.TrimSpace(s.history[0].Content)
}

func (s *CoCreateSession) BuildPlan() (Plan, error) {
	if s == nil || !s.CanStart() {
		return Plan{}, fmt.Errorf("cocreate draft prompt is required")
	}
	return Plan{
		Mode:        ModeCoCreate,
		DisplayName: "Đồng sáng tạo quy hoạch",
		StartPrompt: host.BuildStartPrompt(s.DraftPrompt()),
	}, nil
}
