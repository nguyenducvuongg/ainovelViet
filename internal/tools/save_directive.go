package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/agentcore/schema"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/errs"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// SaveDirectiveTool duy trì các yêu cầu soạn thảo dài hạn của người dùng (chỉ do Điều phối viên nắm giữ).
// Di chuyển đến meta/user_directives.json, tiểu thuyết_context được đưa vào Working_memory.user_directives,
// Tất cả các đại lý phụ tự động nhìn thấy từng chương - không phụ thuộc vào Điều phối viên khi gửi đơn đặt hàng, có hiệu lực trong quá trình nén và khởi động lại.
type SaveDirectiveTool struct {
	store *store.Store
}

func NewSaveDirectiveTool(s *store.Store) *SaveDirectiveTool {
	return &SaveDirectiveTool{store: s}
}

func (t *SaveDirectiveTool) Name() string  { return "save_directive" }
func (t *SaveDirectiveTool) Label() string { return "Lưu hướng dẫn dài hạn" }

func (t *SaveDirectiveTool) Description() string {
	return "Yêu cầu tạo lâu dài của người dùng kiên trì (ví dụ: sau \", tỷ lệ hội thoại tăng lên, \", \" và tiêu đề chương chỉ bằng tiếng Trung \")." +
		"Sau khi lưu, tất cả các tác nhân phụ sẽ thấy từng chương trong Working_memory.user_directives và không cần phải chuyển tiếp lại." +
		"action=add thêm một mục (văn bản là bắt buộc, giữ nguyên ý định của người dùng và có thể được cô đọng một cách thích hợp);" +
		"action=remove xóa theo số sê-ri (bắt buộc phải lập chỉ mục, số sê-ri được hiển thị trong danh sách được trả về lần trước)." +
		"Trả về danh sách đầy đủ được cập nhật. Chỉ các yêu cầu có trạng thái mới được lưu (các mô tả đúng để đọc lại bất kỳ lúc nào);" +
		"Các hướng dẫn tương đối/hành động (chẳng hạn như \" thêm 10 chương vào \") đều bị cấm lưu - công cụ này không cử các tác nhân phụ. Nếu chúng được lưu, điều đó có nghĩa là sẽ không có ai xử tử chúng. Vui lòng sử dụng lộ trình đại lý phụ để xử lý chúng ngay lập tức."
}

// Viết công cụ để vô hiệu hóa đồng thời.
func (t *SaveDirectiveTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *SaveDirectiveTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *SaveDirectiveTool) ActivityDescription(_ json.RawMessage) string { return "Lưu hướng dẫn dài hạn" }

func (t *SaveDirectiveTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("action", schema.Enum("Loại hoạt động", "add", "remove")).Required(),
		schema.Property("text", schema.String("Nội dung bắt buộc (bắt buộc khi thêm): nêu rõ yêu cầu trong một câu và giữ nguyên ý định ban đầu của người dùng")),
		schema.Property("index", schema.Int("Số sê-ri của mục cần xóa (bắt buộc khi xóa, dựa trên 1, xem chỉ mục được danh sách trả về)")),
	)
}

func (t *SaveDirectiveTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Action string `json:"action"`
		Text   string `json:"text"`
		Index  int    `json:"index"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}

	var (
		list []domain.UserDirective
		err  error
	)
	switch a.Action {
	case "add":
		text := strings.TrimSpace(a.Text)
		if text == "" {
			return nil, fmt.Errorf("add yêu cầu văn bản không trống: %w", errs.ErrToolArgs)
		}
		chapter, total := 0, 0
		if progress, perr := t.store.Progress.Load(); perr == nil && progress != nil {
			chapter = progress.NextChapter()
			total = progress.TotalChapters
		}
		list, err = t.store.Directives.Add(domain.UserDirective{
			Text:          text,
			Chapter:       chapter,
			TotalChapters: total,
			CreatedAt:     time.Now().Format(time.RFC3339),
		})
	case "remove":
		if a.Index < 1 {
			return nil, fmt.Errorf("xóa yêu cầu chỉ mục >= 1: %w", errs.ErrToolArgs)
		}
		list, err = t.store.Directives.Remove(a.Index)
	default:
		return nil, fmt.Errorf("unknown action %q: %w", a.Action, errs.ErrToolArgs)
	}
	if err != nil {
		return nil, err
	}

	items := directiveFacts(list)
	return json.Marshal(map[string]any{
		"saved":      true,
		"directives": items,
		"count":      len(items),
	})
}

// chỉ thịFacts chuyển đổi các chỉ thị dài hạn thành các chế độ xem thực tế cho LLM (kết quả của công cụ giống như việc chèn phong bì):
// at_* là ảnh chụp nhanh tiến trình khi được ban hành - hướng dẫn có hiệu lực từ at_chapter trở đi và biểu thức tương đối có thể dựa trên
// at_total_chapters xác định xem nó có thỏa mãn hay không. create_at là thông tin kiểm tra và không nhập LLM.
func directiveFacts(list []domain.UserDirective) []map[string]any {
	items := make([]map[string]any, len(list))
	for i, d := range list {
		items[i] = map[string]any{
			"index":             i + 1,
			"text":              d.Text,
			"at_chapter":        d.Chapter,
			"at_total_chapters": d.TotalChapters,
		}
	}
	return items
}
