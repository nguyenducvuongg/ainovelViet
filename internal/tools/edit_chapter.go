package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/voocel/agentcore/schema"
	agentcoretools "github.com/voocel/agentcore/tools"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/errs"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// EditChapterTool thực hiện thay thế chuỗi điểm cố định trên các bản nháp chương, phù hợp với các tình huống đánh bóng.
// So với việc viết lại toàn bộ chương Draft_chapter, mã thông báo tiết kiệm được hơn 10 lần.
//
// Hợp đồng cam kết: Chỉ có thể sửa đổi bản nháp/{ch:02d}.draft.md, việc sửa đổi trực tiếp các chương/ bị cấm (bản thảo cuối cùng thuộc quyền sở hữu độc quyền của commit_chapter).
// Ngữ nghĩa gốc: bản nháp không tồn tại nhưng các chương thì có → tự động sao chép các chương vào bản nháp làm điểm bắt đầu.
// Kiểm tra quyền sở hữu: Chương phải nằm trong hàng đợi PendingRewrites khi hoàn thành, nếu không chương sẽ bị từ chối.
//
// Công cụ này là một trình bao bọc mỏng của Agentcore.EditTool, với logic tìm kiếm và thay thế (khớp khả năng chịu lỗi đa cấp, đầu ra khác biệt, bảo toàn cuối dòng/BOM)
// Tất cả các triển khai ngược dòng đều được sử dụng lại.
type EditChapterTool struct {
	store *store.Store
	edit  *agentcoretools.EditTool
}

func NewEditChapterTool(s *store.Store) *EditChapterTool {
	return &EditChapterTool{
		store: s,
		edit:  agentcoretools.NewEdit(s.Dir(), nil),
	}
}

func (t *EditChapterTool) Name() string  { return "edit_chapter" }
func (t *EditChapterTool) Label() string { return "Chỉnh sửa chương" }

// ReadOnly khai báo rõ ràng công cụ viết (hợp tác với ConcurrencySafeTool để ngăn chặn việc lập lịch đồng thời).
func (t *EditChapterTool) ReadOnly(_ json.RawMessage) bool { return false }

// ConcurrencySafe nghiêm cấm việc đồng thời một cách rõ ràng: việc chỉnh sửa song song cùng một chương nhiều lần sẽ gây ra tình trạng chạy đua đọc-sửa-ghi.
// Ngay cả khi các chương khác nhau chạy song song thì trình tự điểm kiểm tra sẽ xen kẽ nhau. Nối tiếp thống nhất là ổn định nhất.
func (t *EditChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

// Mô tả hoạt động được sử dụng cho giao diện người dùng/nhật ký để hiển thị mô tả hoạt động của công cụ hiện tại.
func (t *EditChapterTool) ActivityDescription(_ json.RawMessage) string { return "Chỉnh sửa bản thảo chương" }

func (t *EditChapterTool) Description() string {
	return "Thực hiện thay thế chuỗi điểm cố định cho các bản nháp chương (ưu tiên cho các kịch bản đánh bóng, lưu mã thông báo so với bản nháp_chapter viết lại toàn bộ chương)." +
		"Tìm old_string và thay thế nó bằng new_string, yêu cầu khớp chính xác và tính duy nhất (replace_all=true là bắt buộc đối với nhiều kết quả khớp)." +
		"Viết vào bản nháp/{ch}.draft.md; tự động gieo mầm từ các chương nếu bản nháp không tồn tại." +
		"Từ chối thực thi khi chương hoàn thành và không nằm trong hàng đợi PendingRewrites. Chỉ cần một thay đổi cho mỗi cuộc gọi. Vui lòng gọi nhiều lần để thay đổi nhiều lần."
}

func (t *EditChapterTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapter", schema.Int("số chương")).Required(),
		schema.Property("old_string", schema.String("Đoạn chính xác của văn bản gốc cần thay thế, nhiều dòng phải có dấu ngắt dòng; không có thay thế_tất cả, nó phải xuất hiện duy nhất trong bản nháp")).Required(),
		schema.Property("new_string", schema.String("Văn bản mới sau khi thay thế")).Required(),
		schema.Property("replace_all", schema.Bool("Thay thế tất cả các kết quả khớp (mặc định là sai)")),
	)
}

func (t *EditChapterTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter    int    `json:"chapter"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if a.OldString == "" {
		return nil, fmt.Errorf("chuỗi_cũ không được để trống: %w", errs.ErrToolArgs)
	}
	if a.OldString == a.NewString {
		return nil, fmt.Errorf("old_string giống với new_string không sửa đổi: %w", errs.ErrToolArgs)
	}

	// Kiểm tra ghi công: Các chương đã hoàn thành phải được xếp hàng viết lại để tránh làm ô nhiễm bản thảo cuối cùng
	if t.store.Progress.IsChapterCompleted(a.Chapter) {
		progress, _ := t.store.Progress.Load()
		if progress == nil || !slices.Contains(progress.PendingRewrites, a.Chapter) {
			return nil, fmt.Errorf("Chương %d đã được hoàn thành và không có trong hàng chờ PendingRewrites. Nó không thể được chỉnh sửa. Nếu bạn cần sửa đổi nó, trước tiên vui lòng kích hoạt việc viết lại/đánh bóng bằng cách đánh giá của biên tập viên: %w", a.Chapter, errs.ErrToolPrecondition)
		}
	}

	// Hạt giống: Nếu bản nháp không tồn tại, hãy sao chép một bản sao từ các chương làm điểm bắt đầu.
	if err := t.ensureDraft(a.Chapter); err != nil {
		return nil, err
	}

	// Ủy thác Agentcore.EditTool để hoàn tất việc thay thế tìm kiếm
	subArgs, _ := json.Marshal(map[string]any{
		"path":        fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
		"file_path":   fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
		"old_text":    a.OldString,
		"old_string":  a.OldString,
		"new_text":    a.NewString,
		"new_string":  a.NewString,
		"replace_all": a.ReplaceAll,
	})
	result, err := t.edit.Execute(ctx, subArgs)
	if err != nil {
		return nil, fmt.Errorf("apply edit: %w: %w", errs.ErrToolPrecondition, err)
	}

	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(a.Chapter), "edit",
		fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
	); err != nil {
		return nil, fmt.Errorf("checkpoint edit: %w: %w", errs.ErrStoreWrite, err)
	}

	// Hướng dẫn bổ sung: Cho người viết biết các bước tiếp theo để tránh bỏ sót check_consistency/commit_chapter
	var passthrough map[string]any
	if err := json.Unmarshal(result, &passthrough); err != nil {
		return result, nil
	}
	passthrough["chapter"] = a.Chapter
	passthrough["next_step"] = "chỉnh sửa đã được đặt. Nếu vẫn còn sai sót, bạn có thể edit_chapter lại; mặt khác, check_consistency và sau đó commit_chapter"
	return json.Marshal(passthrough)
}

// EnsureDraft đảm bảo rằng Drafts/{ch}.draft.md tồn tại:
//   - Đã có bản nháp → Hoàn trả trực tiếp
//   - Không có bản nháp mà là bản nháp cuối cùng → Sao chép bản nháp cuối cùng vào bản nháp làm điểm bắt đầu sửa đổi (thường gặp trong các kịch bản đánh bóng)
//   - Không có → Báo lỗi, nhắc sử dụng Draft_chapter để tạo bản nháp đầu tiên.
func (t *EditChapterTool) ensureDraft(chapter int) error {
	draft, err := t.store.Drafts.LoadDraft(chapter)
	if err != nil {
		return fmt.Errorf("load draft: %w: %w", errs.ErrStoreRead, err)
	}
	if draft != "" {
		return nil
	}
	text, err := t.store.Drafts.LoadChapterText(chapter)
	if err != nil {
		return fmt.Errorf("load chapter: %w: %w", errs.ErrStoreRead, err)
	}
	if text == "" {
		return fmt.Errorf("Chương %d chưa có bản thảo hoặc bản thảo cuối cùng. Vui lòng gọi Draft_chapter(mode=write, chap=%d) trước để tạo bản nháp đầu tiên: %w", chapter, chapter, errs.ErrToolPrecondition)
	}
	if err := t.store.Drafts.SaveDraft(chapter, text); err != nil {
		return fmt.Errorf("seed draft from chapter: %w: %w", errs.ErrStoreWrite, err)
	}
	return nil
}
