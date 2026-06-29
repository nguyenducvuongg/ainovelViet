package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/agentcore/schema"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// SaveReviewTool lưu kết quả đánh giá của Biên tập viên.
type SaveReviewTool struct {
	store *store.Store
}

func NewSaveReviewTool(store *store.Store) *SaveReviewTool {
	return &SaveReviewTool{store: store}
}

func (t *SaveReviewTool) Name() string { return "save_review" }
func (t *SaveReviewTool) Description() string {
	return "Lưu kết quả đánh giá và cập nhật trạng thái quá trình. phán quyết là một trong những chấp nhận/đánh bóng/viết lại." +
		"Công cụ này thực hiện nội bộ việc đo lường thẻ điểm (có thể nâng cấp phán quyết), cập nhật trực tiếp luồng của Tiến trình và đang chờ xử lý." +
		"Trả về các sự kiện có cấu trúc: Final_verdict/affected_chapters/escalation_reason/next_flow/next_chapter"
}
func (t *SaveReviewTool) Label() string { return "Lưu đánh giá" }

// Các công cụ viết (cập nhật đánh giá/và PendingRewrites/Flow of Progress cùng lúc), cấm hoạt động đồng thời.
func (t *SaveReviewTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *SaveReviewTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *SaveReviewTool) Schema() map[string]any {
	issueSchema := schema.Object(
		schema.Property("type", schema.Enum("khía cạnh vấn đề", "consistency", "character", "pacing", "continuity", "foreshadow", "hook", "aesthetic")).Required(),
		schema.Property("severity", schema.Enum("Mức độ nghiêm trọng", "critical", "error", "warning")).Required(),
		schema.Property("description", schema.String("Mô tả vấn đề")).Required(),
		schema.Property("evidence", schema.String("Bằng chứng: các đoạn văn bản gốc, các sơ đồ cụ thể hoặc dữ liệu trạng thái")).Required(),
		schema.Property("suggestion", schema.String("Đề xuất sửa đổi")),
	)
	dimensionSchema := schema.Object(
		schema.Property("dimension", schema.Enum("Kích thước", "consistency", "character", "pacing", "continuity", "foreshadow", "hook", "aesthetic")).Required(),
		schema.Property("score", schema.Int("Đánh giá (0-100)")).Required(),
		schema.Property("verdict", schema.Enum("Kết luận thứ nguyên (có thể bỏ qua: hệ thống tự động rút ra theo điểm, ≥80 đạt / ≥60 cảnh báo / <60 trượt)", "pass", "warning", "fail")),
		schema.Property("comment", schema.String("Kết luận ngắn gọn về khía cạnh này; mỗi chiều là bắt buộc, thẩm mỹ phải trích dẫn văn bản gốc hoặc số liệu thống kê cụ thể")).Required(),
	)
	return schema.Object(
		schema.Property("chapter", schema.Int("Số chương đã đánh giá (điền số chương mới nhất để đánh giá toàn cầu)")).Required(),
		schema.Property("scope", schema.Enum("Đánh giá phạm vi", "chapter", "global", "arc")).Required(),
		schema.Property("dimensions", schema.Array("Điểm số chiều (một cho mỗi chiều trong số bảy chiều)", dimensionSchema)).Required(),
		schema.Property("issues", schema.Array("Đã tìm thấy vấn đề", issueSchema)).Required(),
		schema.Property("contract_status", schema.Enum("Mức độ hoàn thành hợp đồng chương", "met", "partial", "missed")),
		schema.Property("contract_misses", schema.Array("Các hạng mục hợp đồng chưa hoàn thành hoặc bị vi phạm", schema.String(""))),
		schema.Property("contract_notes", schema.String("Mô tả ngắn gọn về việc thực hiện hợp đồng")),
		schema.Property("verdict", schema.Enum("Xem xét kết luận", "accept", "polish", "rewrite")).Required(),
		schema.Property("summary", schema.String("Tóm tắt đánh giá")).Required(),
		schema.Property("affected_chapters", schema.Array("Danh sách số chương cần viết lại hoặc trau chuốt (bắt buộc khi phán quyết được đánh bóng/viết lại)", schema.Int(""))),
	)
}

func (t *SaveReviewTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var r domain.ReviewEntry
	if err := json.Unmarshal(args, &r); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if r.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0")
	}
	// phán quyết là một hàm thuần túy về điểm số ( ≥80 đạt / ≥60 cảnh báo / <60 trượt), được rút ra một cách xác định từ mã -
	// Ngăn chặn LLM được sử dụng nhiều lần để mang lại tính nhất quán cho việc xác nhận lại. Nó không chỉ loại bỏ sự dư thừa mà còn loại bỏ các tham số mâu thuẫn như "điểm = 85 nhưng đưa ra cảnh báo".
	for i := range r.Dimensions {
		r.Dimensions[i].Verdict = expectedDimensionVerdict(r.Dimensions[i].Score)
	}
	if err := validateReviewEntry(r); err != nil {
		return nil, err
	}

	// Kiểm soát quyền truy cập thẻ điểm - nội tuyến logic nâng cấp của chính sách ban đầu/review.go
	finalVerdict := r.Verdict
	var escalationReason string

	if r.Verdict == "accept" {
		// Kiểm tra tình trạng hợp đồng
		if r.ContractStatus == "missed" {
			finalVerdict = "rewrite"
			escalationReason = "Trạng thái thực hiện hợp đồng bị bỏ sót và được nâng cấp thành viết lại."
		} else if r.ContractStatus == "partial" {
			finalVerdict = "polish"
			escalationReason = "Trạng thái thực hiện hợp đồng là một phần và được nâng cấp lên đánh bóng"
		}
		// kiểm soát truy cập thẻ điểm
		if finalVerdict == "accept" {
			if gate := evaluateScorecardGate(r.Dimensions); gate != "" {
				if strings.Contains(gate, "rewrite") {
					finalVerdict = "rewrite"
				} else {
					finalVerdict = "polish"
				}
				escalationReason = gate
			}
		}
	}

	affected := r.AffectedChapters
	if finalVerdict == "rewrite" || finalVerdict == "polish" {
		if len(affected) == 0 && r.Chapter > 0 {
			affected = []int{r.Chapter}
		}
		if err := t.store.Progress.ValidatePendingRewrites(affected); err != nil {
			return nil, fmt.Errorf("validate pending rewrites: %w", err)
		}
	}

	if err := t.store.World.SaveReview(r); err != nil {
		return nil, fmt.Errorf("save review: %w", err)
	}

	// Cập nhật tiến độ dựa trên phán quyết cuối cùng.
	// Lỗi ghi phải được trả về sớm - điểm kiểm tra đánh giá bổ sung sẽ được bổ sung sau. Nếu nuốt phải lỗi ở đây, Điều phối viên sẽ
	// Xem đã lưu:true nhưng Cửa hàng vẫn ở trạng thái trung gian của Luồng cũ/thiếu PendingRewrites.
	progress, _ := t.store.Progress.Load()
	if finalVerdict == "rewrite" || finalVerdict == "polish" {
		flow := domain.FlowRewriting
		if finalVerdict == "polish" {
			flow = domain.FlowPolishing
		}
		if err := t.store.Progress.SetPendingRewrites(affected, r.Summary); err != nil {
			return nil, fmt.Errorf("set pending rewrites: %w", err)
		}
		if err := t.store.Progress.SetFlow(flow); err != nil {
			return nil, fmt.Errorf("set flow %s: %w", flow, err)
		}
	} else {
		if err := t.store.Progress.SetFlow(domain.FlowWriting); err != nil {
			return nil, fmt.Errorf("set flow writing: %w", err)
		}
	}

	// Đọc ảnh chụp nhanh tiến độ được cập nhật như thực tế
	latest, _ := t.store.Progress.Load()
	nextFlow := string(domain.FlowWriting)
	nextChapter := 0
	if latest != nil {
		nextFlow = string(latest.Flow)
		nextChapter = latest.NextChapter()
	}

	// Thêm điểm kiểm tra
	scope := domain.ChapterScope(r.Chapter)
	if r.Scope == "arc" {
		vol, arc := 0, 0
		if progress != nil {
			vol, arc = progress.CurrentVolume, progress.CurrentArc
		}
		scope = domain.ArcScope(vol, arc)
	}
	artifact := fmt.Sprintf("reviews/%02d.json", r.Chapter)
	if r.Scope == "global" {
		artifact = fmt.Sprintf("reviews/%02d-global.json", r.Chapter)
	}
	if _, err := t.store.Checkpoints.AppendArtifact(scope, "review", artifact); err != nil {
		return nil, fmt.Errorf("checkpoint review: %w", err)
	}

	return json.Marshal(map[string]any{
		"saved":             true,
		"chapter":           r.Chapter,
		"scope":             r.Scope,
		"verdict":           r.Verdict,
		"final_verdict":     finalVerdict,
		"escalation_reason": escalationReason,
		"affected_chapters": affected,
		"issues":            len(r.Issues),
		"next_flow":         nextFlow,
		"next_chapter":      nextChapter,
	})
}

var expectedReviewDimensions = map[string]struct{}{
	"consistency": {},
	"character":   {},
	"pacing":      {},
	"continuity":  {},
	"foreshadow":  {},
	"hook":        {},
	"aesthetic":   {},
}

func validateReviewEntry(r domain.ReviewEntry) error {
	if strings.TrimSpace(r.Scope) == "" {
		return fmt.Errorf("scope is required")
	}
	if strings.TrimSpace(r.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	for _, issue := range r.Issues {
		if strings.TrimSpace(issue.Description) == "" {
			return fmt.Errorf("issue description is required")
		}
		if strings.TrimSpace(issue.Evidence) == "" {
			return fmt.Errorf("issue evidence is required")
		}
	}
	if err := validateDimensions(r.Dimensions); err != nil {
		return err
	}
	if (r.Verdict == "rewrite" || r.Verdict == "polish") && len(r.AffectedChapters) == 0 {
		return fmt.Errorf("affected_chapters is required when verdict=%s", r.Verdict)
	}
	return nil
}

func validateDimensions(dimensions []domain.DimensionScore) error {
	if len(dimensions) != len(expectedReviewDimensions) {
		return fmt.Errorf("dimensions must contain exactly %d entries", len(expectedReviewDimensions))
	}

	seen := make(map[string]struct{}, len(dimensions))
	for _, dim := range dimensions {
		if _, ok := expectedReviewDimensions[dim.Dimension]; !ok {
			return fmt.Errorf("unknown dimension: %s", dim.Dimension)
		}
		if _, ok := seen[dim.Dimension]; ok {
			return fmt.Errorf("duplicate dimension: %s", dim.Dimension)
		}
		seen[dim.Dimension] = struct{}{}
		if dim.Score < 0 || dim.Score > 100 {
			return fmt.Errorf("invalid score for %s: %d", dim.Dimension, dim.Score)
		}
		if strings.TrimSpace(dim.Comment) == "" {
			return fmt.Errorf("dimension comment is required: %s", dim.Dimension)
		}
	}
	return nil
}

func expectedDimensionVerdict(score int) string {
	switch {
	case score >= 80:
		return "pass"
	case score >= 60:
		return "warning"
	default:
		return "fail"
	}
}

// quan trọngDimensions xác định các thứ nguyên quan trọng kích hoạt báo cáo phán quyết.
var criticalDimensions = map[string]struct{}{
	"consistency": {},
	"character":   {},
	"continuity":  {},
}

// đánh giáScorecardGate Kiểm tra xem thẻ điểm có cần được nâng cấp phán quyết hay không.
// Trả về một chuỗi trống để biểu thị không có nâng cấp.
func evaluateScorecardGate(dimensions []domain.DimensionScore) string {
	var criticalFails []string
	var polishIssues []string

	for _, dim := range dimensions {
		_, isCritical := criticalDimensions[dim.Dimension]
		if isCritical && (dim.Verdict == "fail" || dim.Score < 60) {
			criticalFails = append(criticalFails, fmt.Sprintf("%s(%d)", dim.Dimension, dim.Score))
		} else if dim.Verdict == "warning" || (isCritical && dim.Score < 80) {
			polishIssues = append(polishIssues, fmt.Sprintf("%s(%d)", dim.Dimension, dim.Score))
		}
	}

	if len(criticalFails) > 0 {
		return fmt.Sprintf("viết lại: Kích thước chính không đủ tiêu chuẩn %v", criticalFails)
	}
	if len(polishIssues) > 0 {
		return fmt.Sprintf("đánh bóng: Một số kích thước cần được đánh bóng %v", polishIssues)
	}
	return ""
}
