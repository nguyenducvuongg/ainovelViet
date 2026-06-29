package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/voocel/agentcore/schema"
)

// AskUserResponse Kết quả trả lời của người dùng.
type AskUserResponse struct {
	Answers map[string]string // văn bản câu hỏi → câu trả lời do người dùng chọn
	Notes   map[string]string // nội dung câu hỏi → Kiểu nhập tùy chỉnh (khi chọn "Khác")
}

// AskUserHandler chặn và chờ người dùng trả lời, việc này được thực hiện bằng cách tiêm CLI hoặc TUI.
type AskUserHandler func(ctx context.Context, questions []Question) (*AskUserResponse, error)

// Câu hỏi Câu hỏi đơn.
type Question struct {
	Question    string   `json:"question"`
	Header      string   `json:"header"`
	Options     []Option `json:"options"`
	MultiSelect bool     `json:"multiSelect"`
}

// Tùy chọn Tùy chọn.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AskUserTool cho phép LLM đặt câu hỏi có cấu trúc cho người dùng.
type AskUserTool struct {
	mu      sync.RWMutex
	handler AskUserHandler
}

func NewAskUserTool() *AskUserTool {
	return &AskUserTool{}
}

// SetHandler đưa vào các lệnh gọi lại giao diện người dùng, được CLI và TUI triển khai riêng biệt.
func (t *AskUserTool) SetHandler(h AskUserHandler) {
	t.mu.Lock()
	t.handler = h
	t.mu.Unlock()
}

func (t *AskUserTool) Name() string  { return "ask_user" }
func (t *AskUserTool) Label() string { return "Hỏi người dùng" }

// Công cụ tương tác: Các khối chờ người dùng trả lời, rõ ràng là không thể lên lịch đồng thời.
func (t *AskUserTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *AskUserTool) ConcurrencySafe(_ json.RawMessage) bool { return false }
func (t *AskUserTool) Description() string {
	return "Khi thông tin nhu cầu không đầy đủ và thông tin còn thiếu sẽ ảnh hưởng đáng kể đến định hướng quy hoạch, hãy hỏi người dùng 1-4 câu hỏi có cấu trúc. Mỗi câu hỏi phải có tiêu đề, câu hỏi và 2-4 lựa chọn; người dùng có thể chọn các mục cài sẵn hoặc thêm chúng một cách tự do. Kết quả trả về là một bản tóm tắt bằng tiếng Trung có thể đọc trực tiếp với định dạng tương tự: Câu trả lời của người dùng: [Độ dài] Truyện dài; [Tập trung] Nâng cấp cốt truyện (Bổ sung: Không cần hậu cung). Chỉ sử dụng nó khi bạn không thể đánh giá ổn định độ dài, trọng tâm chính, các ràng buộc chính hoặc các ưu tiên rõ ràng; không ném tất cả các câu hỏi cho người dùng mà họ có thể tự suy luận một cách hợp lý."
}

func (t *AskUserTool) Schema() map[string]any {
	option := schema.Object(
		schema.Property("label", schema.String("Tùy chọn hiển thị văn bản (1-5 từ)")).Required(),
		schema.Property("description", schema.String("Mô tả ý nghĩa tùy chọn")).Required(),
	)
	question := schema.Object(
		schema.Property("question", schema.String("toàn văn câu hỏi")).Required(),
		schema.Property("header", schema.String("Nhãn ngắn (tối đa 12 ký tự)")).Required(),
		schema.Property("options", schema.Array("2-4 lựa chọn", option)).Required(),
		schema.Property("multiSelect", schema.Bool("Có cho phép nhiều lựa chọn hay không")),
	)
	return schema.Object(
		schema.Property("questions", schema.Array("1-4 câu hỏi", question)).Required(),
	)
}

type askUserArgs struct {
	Questions []Question `json:"questions"`
}

func (t *AskUserTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a askUserArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if err := validateQuestions(a.Questions); err != nil {
		return json.Marshal(fmt.Sprintf("Xác minh tham số không thành công: %s", err))
	}

	t.mu.RLock()
	h := t.handler
	t.mu.RUnlock()

	if h == nil {
		return json.Marshal("Môi trường hiện tại không hỗ trợ các truy vấn tương tác, vui lòng sử dụng phán đoán của riêng bạn và tiếp tục.")
	}

	resp, err := h(ctx, a.Questions)
	if err != nil {
		return json.Marshal(fmt.Sprintf("Tương tác người dùng không thành công: %s. Hãy sử dụng phán đoán của riêng bạn và tiến hành.", err))
	}

	return json.Marshal(formatAnswers(a.Questions, resp))
}

func validateQuestions(questions []Question) error {
	if len(questions) == 0 {
		return fmt.Errorf("Cần có ít nhất một câu hỏi")
	}
	if len(questions) > 4 {
		return fmt.Errorf("Tối đa 4 câu hỏi, hiện %d", len(questions))
	}
	for i, q := range questions {
		if q.Question == "" {
			return fmt.Errorf("Câu hỏi %d: Nội dung câu hỏi không được để trống", i+1)
		}
		if q.Header == "" {
			return fmt.Errorf("Sự cố %d: tiêu đề không thể trống", i+1)
		}
		if utf8.RuneCountInString(q.Header) > 12 {
			return fmt.Errorf("Sự cố %d: tiêu đề %q vượt quá 12 ký tự", i+1, q.Header)
		}
		if len(q.Options) < 2 || len(q.Options) > 4 {
			return fmt.Errorf("Câu %d: Cần 2-4 phương án, hiện tại là %d", i+1, len(q.Options))
		}
		for j, opt := range q.Options {
			if opt.Label == "" {
				return fmt.Errorf("Sự cố Tùy chọn %d %d: nhãn không được để trống", i+1, j+1)
			}
			if opt.Description == "" {
				return fmt.Errorf("Sự cố Tùy chọn %d %d: mô tả không được để trống", i+1, j+1)
			}
		}
	}
	return nil
}

func formatAnswers(questions []Question, resp *AskUserResponse) string {
	if resp == nil || len(resp.Answers) == 0 {
		return "Người dùng chưa đưa ra câu trả lời, vui lòng sử dụng phán đoán của riêng bạn và tiếp tục."
	}
	var parts []string
	for _, q := range questions {
		answer, ok := resp.Answers[q.Question]
		if !ok {
			continue
		}
		entry := fmt.Sprintf("[%s] %s", q.Header, answer)
		if note, hasNote := resp.Notes[q.Question]; hasNote {
			entry += "(Bổ sung:" + note + "）"
		}
		parts = append(parts, entry)
	}
	return fmt.Sprintf("Câu trả lời của người dùng: %s", strings.Join(parts, "；"))
}
