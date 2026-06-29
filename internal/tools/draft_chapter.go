package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
)

// DraftChapterTool viết toàn bộ bản nháp của chương, thay thế quy trình write_scene + Polish_chapter cũ.
// Người đại diện quyết định độc lập xem nên viết xong một lần hay tiếp tục viết theo đợt.
type DraftChapterTool struct {
	store *store.Store
}

func NewDraftChapterTool(store *store.Store) *DraftChapterTool {
	return &DraftChapterTool{store: store}
}

func (t *DraftChapterTool) Name() string { return "draft_chapter" }
func (t *DraftChapterTool) Description() string {
	return "Viết nội dung chương. mode=write ghi đè toàn bộ chương, mode=append thêm vào bản nháp hiện có (tiếp tục/sửa đổi)"
}
func (t *DraftChapterTool) Label() string { return "viết chương" }

// Các công cụ viết cấm đồng thời (điều kiện chạy đua đọc-sửa-ghi).
func (t *DraftChapterTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *DraftChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *DraftChapterTool) Schema() map[string]any {
	// Dấu chế độ được yêu cầu là để tương thích với việc gọi công cụ nghiêm ngặt của OpenAI——chế độ nghiêm ngặt
	// Tất cả các thuộc tính được yêu cầu phải có trong danh sách bắt buộc. Chế độ "bỏ qua và viết" ban đầu
	// Hành vi mặc định hiện yêu cầu mô hình chuyển rõ ràng mode="write" và nhánh mặc định của Thực thi vẫn không thay đổi.
	return schema.Object(
		schema.Property("chapter", schema.Int("số chương")).Required(),
		schema.Property("content", schema.String("văn bản chương")).Required(),
		schema.Property("mode", schema.Enum("chế độ viết", "write", "append")).Required(),
	)
}

// StrictSchema cho phép gọi công cụ nghiêm ngặt của OpenAI để mô hình phải tuân thủ nghiêm ngặt
// Lược đồ: Tất cả các trường bắt buộc là bắt buộc, các đối số không thể là "EOT sớm" và các đối tượng trống xuất hiện.
// litellm truyền trường nghiêm ngặt một cách minh bạch; các chương trình phụ trợ được hỗ trợ như OpenAI / xAI sẽ thực thi nó và các chương trình phụ trợ khác sẽ
// Các trường không xác định sẽ bị bỏ qua theo quy ước HTTP/JSON. Anthropic/Gemini/Bedrock lấy link chuyển đổi riêng
// Đương nhiên bạn sẽ không nhìn thấy trường này.
func (t *DraftChapterTool) StrictSchema() bool { return true }

func (t *DraftChapterTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter int    `json:"chapter"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if a.Content == "" {
		return nil, fmt.Errorf("content must not be empty: %w", errs.ErrToolArgs)
	}
	if err := t.store.Progress.ValidateChapterWork(a.Chapter); err != nil {
		return nil, err
	}
	if err := EnsureChapterExpanded(t.store, a.Chapter); err != nil {
		return nil, err
	}
	if t.store.Progress.IsChapterCompleted(a.Chapter) {
		// Đường dẫn Ba Lan/viết lại: Chương đã hoàn thành nhưng vẫn ở trạng thái chờ_rewrites, cho phép ghi đè bản nháp
		progress, _ := t.store.Progress.Load()
		inRewriteQueue := progress != nil && slices.Contains(progress.PendingRewrites, a.Chapter)
		if !inRewriteQueue {
			return json.Marshal(map[string]any{
				"chapter":   a.Chapter,
				"skipped":   true,
				"completed": true,
				"reason":    fmt.Sprintf("Chương %d đã được gửi và không thể ghi đè.", a.Chapter),
			})
		}
	}
	if err := t.store.Progress.StartChapter(a.Chapter); err != nil {
		return nil, fmt.Errorf("mark chapter in progress: %w", err)
	}

	switch a.Mode {
	case "append":
		if err := t.store.Drafts.AppendDraft(a.Chapter, a.Content); err != nil {
			return nil, fmt.Errorf("append draft: %w", err)
		}
		full, err := t.store.Drafts.LoadDraft(a.Chapter)
		if err != nil {
			return nil, fmt.Errorf("load draft after append: %w", err)
		}
		if _, err := t.store.Checkpoints.AppendArtifact(
			domain.ChapterScope(a.Chapter), "draft",
			fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
		); err != nil {
			return nil, fmt.Errorf("checkpoint draft: %w", err)
		}
		return json.Marshal(map[string]any{
			"written":    true,
			"chapter":    a.Chapter,
			"mode":       "append",
			"word_count": utf8.RuneCountInString(full),
			"next_step":  "Đầu tiên read_chapter(source=draft) đọc lại bản nháp, sau đó gọi check_consistency và cuối cùng là commit_chapter",
		})
	default: // write
		if err := t.store.Drafts.SaveDraft(a.Chapter, a.Content); err != nil {
			return nil, fmt.Errorf("save draft: %w", err)
		}
		if _, err := t.store.Checkpoints.AppendArtifact(
			domain.ChapterScope(a.Chapter), "draft",
			fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
		); err != nil {
			return nil, fmt.Errorf("checkpoint draft: %w", err)
		}
		return json.Marshal(map[string]any{
			"written":    true,
			"chapter":    a.Chapter,
			"mode":       "write",
			"word_count": utf8.RuneCountInString(a.Content),
			"next_step":  "Đầu tiên read_chapter(source=draft) đọc lại bản nháp, sau đó gọi check_consistency và cuối cùng là commit_chapter",
		})
	}
}
