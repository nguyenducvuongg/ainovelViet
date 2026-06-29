package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
)

// ReopenBookTool mở lại cuốn sách đã hoàn thành ở trạng thái làm lại (chỉ do Điều phối viên nắm giữ).
// Sau khi hoàn thành cuốn sách, CompletePhaseGate chặn tất cả hoạt động phân phối tác nhân phụ và người dùng không thể làm lại các chương đã viết.
// Công cụ này không phải là tác nhân phụ và giai đoạn hoàn chỉnh có thể điều chỉnh được: nó chuyển giai đoạn trở lại viết và chương mục tiêu một cách nguyên tử.
// PendingRewrites, flow=rewriting, sau đó Flow Router sẽ cử người viết theo hàng đợi làm lại hiện có để viết lại từng chương,
// Khi hết hàng đợi, commit_chapter sẽ tự động kết thúc lại và hoàn thành. Cổng/Bộ định tuyến/chỉnh sửa/cam kết và logic nặng không cần phải thay đổi.
type ReopenBookTool struct {
	store *store.Store
}

func NewReopenBookTool(s *store.Store) *ReopenBookTool {
	return &ReopenBookTool{store: s}
}

func (t *ReopenBookTool) Name() string  { return "reopen_book" }
func (t *ReopenBookTool) Label() string { return "Khởi động lại và làm lại" }

func (t *ReopenBookTool) Description() string {
	return "Mở lại cuốn sách đã hoàn thành (giai đoạn=hoàn thành) ở trạng thái làm lại, trạng thái này được sử dụng khi người dùng yêu cầu viết lại/đánh bóng các chương nhất định sau khi hoàn thành cuốn sách." +
		"chương là số chương đã hoàn thành được làm lại; sau cuộc gọi, những chương này sẽ được đưa vào hàng viết lại, người chủ trì sẽ cử người viết viết lại từng chương. Sau khi tất cả các thay đổi được hoàn thành, chúng sẽ tự động được hoàn thành lại." +
		"Chỉ sử dụng nó khi cuốn sách đã hoàn chỉnh và người dùng yêu cầu sửa đổi các chương đã viết một cách rõ ràng; nếu người dùng muốn thêm ô/mở rộng độ dài thì không phải làm lại nên không sử dụng công cụ này."
}

// Viết công cụ để vô hiệu hóa đồng thời.
func (t *ReopenBookTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *ReopenBookTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *ReopenBookTool) ActivityDescription(_ json.RawMessage) string { return "Mở lại toàn bộ cuốn sách và làm lại nó" }

func (t *ReopenBookTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapters", schema.Array("Danh sách số chương đã hoàn thành cần làm lại (ít nhất một chương)", schema.Int(""))).Required(),
		schema.Property("reason", schema.String("Lý do làm lại (tùy chọn, chẳng hạn như \" làm sạch các ký tự đặc biệt \")")),
	)
}

func (t *ReopenBookTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapters []int  `json:"chapters"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if len(a.Chapters) == 0 {
		return nil, fmt.Errorf("các chương không được để trống, bạn cần ghi rõ các chương cần làm lại: %w", errs.ErrToolArgs)
	}

	progress, err := t.store.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("load progress: %w: %w", errs.ErrStoreRead, err)
	}
	if progress == nil {
		return nil, fmt.Errorf("tiến trình chưa được khởi tạo: %w", errs.ErrToolPrecondition)
	}
	// Chỉ những chương đã được viết mới có thể được làm lại; số chương không có trong bộ sưu tập đã hoàn chỉnh sẽ được tiếp tục/vượt quá giới hạn và người dùng rõ ràng bị từ chối hướng dẫn điều chỉnh độ dài.
	var invalid []int
	for _, ch := range a.Chapters {
		if !slices.Contains(progress.CompletedChapters, ch) {
			invalid = append(invalid, ch)
		}
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("Chương %v chưa được viết. Mở lại chỉ có thể làm lại các chương đã hoàn thành (vui lòng điều chỉnh độ dài cho các cốt truyện mới/mở rộng): %w", invalid, errs.ErrToolPrecondition)
	}

	// Giai đoạn kiểm tra trước được thực hiện bên trong cửa hàng. Mở lại (chỉ có thể điều chỉnh hoàn thành).
	if err := t.store.Progress.Reopen(a.Chapters, a.Reason); err != nil {
		return nil, fmt.Errorf("reopen: %w: %w", errs.ErrStoreWrite, err)
	}

	// điểm kiểm tra: Đối xứng với Complete_book (GlobalScope + meta/progress.json).
	if _, err := t.store.Checkpoints.AppendArtifact(domain.GlobalScope(), "reopen", "meta/progress.json"); err != nil {
		return nil, fmt.Errorf("checkpoint reopen: %w: %w", errs.ErrStoreWrite, err)
	}

	return json.Marshal(map[string]any{
		"reopened":         true,
		"phase":            string(domain.PhaseWriting),
		"pending_rewrites": a.Chapters,
		"next_step":        "Đã mở lại và thêm mục tiêu vào hàng đợi. Hãy chờ lệnh Host cử người viết làm lại từng chương; nó sẽ được hoàn thành tự động sau khi tất cả các thay đổi được hoàn thành.",
	})
}
