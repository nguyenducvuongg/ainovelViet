package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
)

// CommitChapterTool Gửi một chương: Tải văn bản → Lưu bản nháp cuối cùng → Tạo bản tóm tắt → Cập nhật trạng thái → Tiến trình cập nhật.
type CommitChapterTool struct {
	store     *store.Store
	rulesOpts rules.LoadOptions // Không bắt buộc; Rule_violations không được tạo khi LoadOptions trống
}

func NewCommitChapterTool(store *store.Store) *CommitChapterTool {
	return &CommitChapterTool{store: store}
}

// WithRules đưa vào các tùy chọn tải quy tắc của người dùng để Rule_violations đi kèm với kết quả kiểm tra quy tắc của người dùng.
// Khi phương pháp này không được gọi thì chỉ có Lint dòng dưới cùng tích hợp sẵn (kiểm tra dư lượng cơ chế, luôn bật) được thực hiện.
func (t *CommitChapterTool) WithRules(opts rules.LoadOptions) *CommitChapterTool {
	t.rulesOpts = opts
	return t
}

// commitOutput nhúng các trường mở rộng trên domain.CommitResult để giữ cho gói miền độc lập với các quy tắc.
// Vì các trường nhúng được trình sắp xếp JSON quảng bá nên kết quả tuần tự hóa tương đương với cấu trúc phẳng.
type commitOutput struct {
	domain.CommitResult
	RuleViolations []rules.Violation `json:"rule_violations,omitempty"`
}

func (t *CommitChapterTool) Name() string { return "commit_chapter" }
func (t *CommitChapterTool) Description() string {
	return "Gửi bản thảo chương cuối cùng. Tải văn bản của bản nháp và lưu dưới dạng bản nháp cuối cùng, cập nhật dòng thời gian, điềm báo, mối quan hệ, trạng thái nhân vật và tiến trình." +
		"Trả về các sự kiện có cấu trúc: next_chapter/review_required/arc_end/volume_end/needs_expansion/book_complete/flow, v.v."
}
func (t *CommitChapterTool) Label() string { return "Gửi chương" }

// Các công cụ viết (hoạt động nguyên tử trên nhiều miền: bản nháp→bản thảo cuối cùng→tóm tắt→tiến trình→điểm kiểm tra), cấm đồng thời.
func (t *CommitChapterTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *CommitChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *CommitChapterTool) Schema() map[string]any {
	timelineSchema := schema.Object(
		schema.Property("time", schema.String("giờ kể chuyện")).Required(),
		schema.Property("event", schema.String("mô tả sự kiện")).Required(),
		schema.Property("characters", schema.Array("liên quan đến vai trò", schema.String(""))),
	)
	foreshadowSchema := schema.Object(
		schema.Property("id", schema.String("ID báo trước")).Required(),
		schema.Property("action", schema.Enum("vận hành", "plant", "advance", "resolve")).Required(),
		schema.Property("description", schema.String("Mô tả điềm báo (chỉ bắt buộc đối với thực vật)")),
	)
	relationshipSchema := schema.Object(
		schema.Property("character_a", schema.String("Vai trò A")).Required(),
		schema.Property("character_b", schema.String("Vai trò B")).Required(),
		schema.Property("relation", schema.String("Mô tả mối quan hệ hiện tại")).Required(),
	)
	stateChangeSchema := schema.Object(
		schema.Property("entity", schema.String("Tên vai trò hoặc tên thực thể")).Required(),
		schema.Property("field", schema.String("thay đổi thuộc tính")).Required(),
		schema.Property("old_value", schema.String("giá trị trước khi thay đổi")),
		schema.Property("new_value", schema.String("giá trị đã thay đổi")).Required(),
		schema.Property("reason", schema.String("Lý do thay đổi")),
	)
	feedbackSchema := schema.Object(
		schema.Property("deviation", schema.String("Mô tả sai lệch so với phác thảo")).Required(),
		schema.Property("suggestion", schema.String("Đề xuất điều chỉnh cho các phác thảo tiếp theo")).Required(),
	)
	feedbackSchema["description"] = "Đối tượng gợi ý cho những phác thảo tiếp theo; phải truyền trực tiếp đối tượng JSON, không truyền JSON được xâu chuỗi"
	return schema.Object(
		schema.Property("chapter", schema.Int("số chương")).Required(),
		schema.Property("summary", schema.String("Tóm tắt chương này (trong vòng 200 từ)")).Required(),
		schema.Property("characters", schema.Array("Tên nhân vật xuất hiện trong chap này", schema.String(""))).Required(),
		schema.Property("key_events", schema.Array("Các sự kiện chính trong chương này", schema.String(""))).Required(),
		schema.Property("timeline_events", schema.Array("Dòng thời gian sự kiện trong chương này", timelineSchema)),
		schema.Property("foreshadow_updates", schema.Array("Hoạt động báo trước", foreshadowSchema)),
		schema.Property("relationship_changes", schema.Array("thay đổi mối quan hệ", relationshipSchema)),
		schema.Property("state_changes", schema.Array("Thay đổi trạng thái vai trò/thực thể", stateChangeSchema)),
		schema.Property("cast_intros", schema.Array("Giới thiệu về các nhân vật phụ được giới thiệu lần đầu trong chương này và có thể xuất hiện lại trong tương lai (không bao gồm nhân vật chính và các nhân vật hiện có trong character.json)", schema.Object(
			schema.Property("name", schema.String("Tên nhân vật")).Required(),
			schema.Property("brief_role", schema.String("Định vị một câu (chẳng hạn như: chủ quán trọ/con bạc)")).Required(),
		))),
		schema.Property("hook_type", schema.Enum("Loại móc cuối chương", "crisis", "mystery", "desire", "emotion", "choice")),
		schema.Property("dominant_strand", schema.Enum("Dòng tường thuật chính của chương này", "quest", "fire", "constellation")),
		schema.Property("feedback", feedbackSchema),
	)
}

func (t *CommitChapterTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter             int                        `json:"chapter"`
		Summary             string                     `json:"summary"`
		Characters          []string                   `json:"characters"`
		KeyEvents           []string                   `json:"key_events"`
		TimelineEvents      []domain.TimelineEvent     `json:"timeline_events"`
		ForeshadowUpdates   []domain.ForeshadowUpdate  `json:"foreshadow_updates"`
		RelationshipChanges []domain.RelationshipEntry `json:"relationship_changes"`
		StateChanges        []domain.StateChange       `json:"state_changes"`
		CastIntros          []domain.CastIntro         `json:"cast_intros"`
		HookType            string                     `json:"hook_type"`
		DominantStrand      string                     `json:"dominant_strand"`
		Feedback            *domain.OutlineFeedback    `json:"feedback"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if t.store.Progress.IsChapterCompleted(a.Chapter) {
		// Dọn sạch các PendingCommit còn lại có thể xảy ra (sự cố xảy ra sau ProgressMarked và trước ClearPendingCommit)
		if pending, _ := t.store.Signals.LoadPendingCommit(); pending != nil && pending.Chapter == a.Chapter {
			_ = t.store.Signals.ClearPendingCommit()
		}
		// Đường dẫn Ba Lan/viết lại: Các chương đã hoàn thành nhưng vẫn ở trạng thái chờ_rewrites, cho phép ghi đè và rút hết hàng đợi
		progress, _ := t.store.Progress.Load()
		if progress != nil && slices.Contains(progress.PendingRewrites, a.Chapter) {
			return t.executeRewriteCommit(a.Chapter, a.Summary, a.Characters, a.KeyEvents,
				a.HookType, a.DominantStrand, progress)
		}
		return t.buildSkipResult(a.Chapter, progress)
	}
	existingPending, err := t.store.Signals.LoadPendingCommit()
	if err != nil {
		return nil, fmt.Errorf("load pending commit: %w: %w", errs.ErrStoreRead, err)
	}
	if existingPending != nil && existingPending.Chapter != a.Chapter {
		return nil, fmt.Errorf("Có chương chưa được khôi phục: Chương %d (Giai đoạn %s), vui lòng khôi phục hoặc gửi lại chương này trước: %w", existingPending.Chapter, existingPending.Stage, errs.ErrToolConflict)
	}
	if err := t.store.Progress.ValidateChapterWork(a.Chapter); err != nil {
		// Xung đột hàng đợi vẫn như cũ (với phân loại ErrToolConflict); các lỗi IO khác thuộc Điều kiện tiên quyết.
		if errors.Is(err, errs.ErrToolConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("Hiện không được phép gửi bài cho các chương: %w: %w", errs.ErrToolPrecondition, err)
	}

	// Chế độ phân cấp chặn xuyên biên giới: phải đi trước bất kỳ thao tác ghi nào, nếu không, cam kết xuyên biên giới sẽ xóa tệp chương, tóm tắt,
	// Tiến bộ đã thay đổi. Ranh giới được ghép kênh để tính toán tín hiệu cung/âm lượng ở bước 6b bên dưới.
	var boundary *store.ArcBoundary
	if progress, perr := t.store.Progress.Load(); perr == nil && progress != nil && progress.Layered {
		b, bErr := t.store.Outline.CheckArcBoundary(a.Chapter)
		if bErr != nil {
			return nil, fmt.Errorf("Chương không phát hiện được ranh giới hồ quang=%d: %w: %w", a.Chapter, errs.ErrStoreRead, bErr)
		}
		if b == nil {
			return nil, fmt.Errorf(
				"Chương %d không nằm trong phạm vi đề cương phân cấp: viết trước tiên phải mở rộng_arc hoặc nối thêm_volume để thêm tập; nếu cuốn sách đã hoàn thành, vui lòng gọi save_foundation type=complete_book: %w",
				a.Chapter, errs.ErrToolPrecondition)
		}
		boundary = b
	}

	// 1. Tải văn bản chương
	content, wordCount, err := t.store.Drafts.LoadChapterContent(a.Chapter)
	if err != nil {
		return nil, fmt.Errorf("load chapter content: %w: %w", errs.ErrStoreRead, err)
	}
	if content == "" {
		return nil, fmt.Errorf("no content found for chapter %d: %w", a.Chapter, errs.ErrToolPrecondition)
	}

	now := time.Now().Format(time.RFC3339)
	pending := domain.PendingCommit{
		Chapter:        a.Chapter,
		Stage:          domain.CommitStageStarted,
		Summary:        a.Summary,
		HookType:       a.HookType,
		DominantStrand: a.DominantStrand,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("save pending commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 2. Lưu bản nháp cuối cùng
	if err := t.store.Drafts.SaveFinalChapter(a.Chapter, content); err != nil {
		return nil, fmt.Errorf("save final chapter: %w: %w", errs.ErrStoreWrite, err)
	}

	// 3. Lưu tóm tắt
	summary := domain.ChapterSummary{
		Chapter:    a.Chapter,
		Summary:    a.Summary,
		Characters: a.Characters,
		KeyEvents:  a.KeyEvents,
	}
	if err := t.store.Summaries.SaveSummary(summary); err != nil {
		return nil, fmt.Errorf("save summary: %w: %w", errs.ErrStoreWrite, err)
	}

	// 4. Cập nhật trạng thái tăng dần
	if len(a.TimelineEvents) > 0 {
		for i := range a.TimelineEvents {
			a.TimelineEvents[i].Chapter = a.Chapter
		}
		if err := t.store.World.AppendTimelineEvents(a.TimelineEvents); err != nil {
			return nil, fmt.Errorf("append timeline: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.ForeshadowUpdates) > 0 {
		if err := t.store.World.UpdateForeshadow(a.Chapter, a.ForeshadowUpdates); err != nil {
			return nil, fmt.Errorf("update foreshadow: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.RelationshipChanges) > 0 {
		for i := range a.RelationshipChanges {
			a.RelationshipChanges[i].Chapter = a.Chapter
		}
		if err := t.store.World.UpdateRelationships(a.RelationshipChanges); err != nil {
			return nil, fmt.Errorf("update relationships: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.StateChanges) > 0 {
		for i := range a.StateChanges {
			a.StateChanges[i].Chapter = a.Chapter
		}
		if err := t.store.World.AppendStateChanges(a.StateChanges); err != nil {
			return nil, fmt.Errorf("append state changes: %w: %w", errs.ErrStoreWrite, err)
		}
	}

	// 4b. Tích lũy danh sách nhân vật phụ: các nhân vật không phải cốt lõi xuất hiện trong chương này được nhập vào cast_ledger để Novel_context gọi lại.
	// Khi thất bại, nó chỉ cảnh báo chứ không chặn cam kết - danh sách là dữ liệu thứ cấp và có thể tự phục hồi thông qua cam kết của chương tiếp theo.
	if len(a.Characters) > 0 {
		coreNames := loadCoreCharacterNameSet(t.store)
		if err := t.store.Cast.MergeAppearances(a.Chapter, a.Characters, a.CastIntros, coreNames); err != nil {
			slog.Warn("Danh sách diễn viên phụ không được tích lũy và bị bỏ qua.", "module", "commit", "chapter", a.Chapter, "err", err)
		}
	}

	pending.Stage = domain.CommitStageStateApplied
	pending.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("update pending commit stage: %w: %w", errs.ErrStoreWrite, err)
	}

	// 5. Cập nhật tiến độ
	if err := t.store.Progress.MarkChapterComplete(a.Chapter, wordCount, a.HookType, a.DominantStrand); err != nil {
		return nil, fmt.Errorf("mark chapter complete: %w: %w", errs.ErrStoreWrite, err)
	}

	// 6. Xác định xem có cần xem xét lại không
	progress, err := t.store.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("load progress: %w: %w", errs.ErrStoreRead, err)
	}
	completedCount := 0
	if progress != nil {
		completedCount = len(progress.CompletedChapters)
	}

	// 6b. Tín hiệu âm lượng/cung ở chế độ dài: ranh giới đã được kiểm tra trước ở lối vào và được đảm bảo không bằng 0 khi được phân lớp
	var arcEnd, volumeEnd, needsExpansion, needsNewVolume bool
	var vol, arc, nextVol, nextArc int
	if progress != nil && progress.Layered && boundary != nil {
		arcEnd = boundary.IsArcEnd
		volumeEnd = boundary.IsVolumeEnd
		vol = boundary.Volume
		arc = boundary.Arc
		needsExpansion = boundary.NeedsExpansion
		needsNewVolume = boundary.NeedsNewVolume
		nextVol = boundary.NextVolume
		nextArc = boundary.NextArc
		_ = t.store.Progress.UpdateVolumeArc(vol, arc)
	}

	var reviewRequired bool
	var reviewReason string
	if progress != nil && progress.Layered {
		reviewRequired, reviewReason = domain.ShouldArcReview(arcEnd, volumeEnd, vol, arc)
	} else {
		reviewRequired, reviewReason = domain.ShouldReview(completedCount)
	}

	// 7. Xây dựng tín hiệu có cấu trúc
	result := domain.CommitResult{
		Chapter:        a.Chapter,
		Committed:      true,
		WordCount:      wordCount,
		NextChapter:    a.Chapter + 1,
		ReviewRequired: reviewRequired,
		ReviewReason:   reviewReason,
		HookType:       a.HookType,
		DominantStrand: a.DominantStrand,
		Feedback:       a.Feedback,
		ArcEnd:         arcEnd,
		VolumeEnd:      volumeEnd,
		Volume:         vol,
		Arc:            arc,
		NeedsExpansion: needsExpansion,
		NeedsNewVolume: needsNewVolume,
		NextVolume:     nextVol,
		NextArc:        nextArc,
	}

	// 8. Xác định trạng thái hoàn thành: viết chương cuối không phân lớp/Chương cuối của tập cuối có phân lớp → Đánh dấu đã hoàn thành
	if t.applyCompletion(&result, progress) {
		result.BookComplete = true
	}
	if p, _ := t.store.Progress.Load(); p != nil {
		result.Flow = string(p.Flow)
	}

	pending.Stage = domain.CommitStageProgressMarked
	pending.Result = &result
	pending.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("update pending commit result: %w: %w", errs.ErrStoreWrite, err)
	}

	// 9. Xóa trạng thái trung gian tiến độ
	if err := t.store.Progress.ClearInProgress(); err != nil {
		return nil, fmt.Errorf("clear in-progress: %w: %w", errs.ErrStoreWrite, err)
	}
	if err := t.store.Signals.ClearPendingCommit(); err != nil {
		return nil, fmt.Errorf("clear pending commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 10. Thêm điểm kiểm tra
	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(a.Chapter), "commit",
		fmt.Sprintf("chapters/%02d.md", a.Chapter),
	); err != nil {
		return nil, fmt.Errorf("checkpoint commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 11. Kiểm tra quy tắc cơ học (chỉ trả về dữ kiện, không chặn)
	violations := t.checkRules(content, wordCount)
	return json.Marshal(commitOutput{CommitResult: result, RuleViolations: violations})
}

// checkRules thực hiện kiểm tra cơ học đối với văn bản chương: dòng cuối cùng của sản phẩm tích hợp Lint (cơ chế còn lại, luôn được thực thi)
// + Kiểm tra quy tắc người dùng (khi các quy tắcOpts hoàn toàn trống, trình tải trả về các lớp trống và trình kiểm tra trả về con số không).
func (t *CommitChapterTool) checkRules(text string, wordCount int) []rules.Violation {
	violations := rules.Lint(text)
	bundle := rules.Merge(rules.Load(t.rulesOpts))
	return append(violations, rules.Check(text, wordCount, bundle.Structured)...)
}

// execRewriteCommit xử lý việc gửi các chương đã được đánh bóng/viết lại: bao gồm bản thảo và tóm tắt cuối cùng, cập nhật số từ, thoát hàng đợi.
// Bỏ qua tất cả việc bổ sung trạng thái thế giới (dòng thời gian/báo trước/mối quan hệ/state_changes) và phát hiện ranh giới vòng cung,
// Những điều này đã được áp dụng tại thời điểm nộp chương ban đầu.
func (t *CommitChapterTool) executeRewriteCommit(
	chapter int,
	summary string,
	characters, keyEvents []string,
	hookType, dominantStrand string,
	progress *domain.Progress,
) (json.RawMessage, error) {
	// 1. Tải văn bản đã được đánh bóng
	content, wordCount, err := t.store.Drafts.LoadChapterContent(chapter)
	if err != nil {
		return nil, fmt.Errorf("rewrite: load chapter content: %w: %w", errs.ErrStoreRead, err)
	}
	if content == "" {
		return nil, fmt.Errorf("no content found for chapter %d: %w", chapter, errs.ErrToolPrecondition)
	}

	// 2. Xác minh cứng: bản nháp giống hệt bản thảo cuối cùng hiện tại → bị đánh giá là chưa thực sự trau chuốt/viết lại (người viết đã bỏ qua bản nháp_chapter)
	// Từ chối cam kết và buộc người viết trước tiên gọi Draft_chapter(mode=write) để viết phiên bản mới.
	existingFinal, _ := t.store.Drafts.LoadChapterText(chapter)
	if existingFinal != "" && existingFinal == content {
		mode := "viết lại"
		if progress != nil && progress.Flow == domain.FlowPolishing {
			mode = "đánh bóng"
		}
		return nil, fmt.Errorf("Bản nháp của Chương %d có nội dung giống hệt như các chương và không phát hiện thấy thay đổi nào đối với %s. Vui lòng điều chỉnh Draft_chapter(mode=write, chap=%d) trước để viết văn bản mới sau %s, sau đó là commit_chapter: %w",
			chapter, mode, chapter, mode, errs.ErrToolPrecondition)
	}

	// 3. Bìa bản thảo cuối cùng
	if err := t.store.Drafts.SaveFinalChapter(chapter, content); err != nil {
		return nil, fmt.Errorf("rewrite: save final chapter: %w: %w", errs.ErrStoreWrite, err)
	}

	// 3. Tóm tắt bảo hiểm
	if err := t.store.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter:    chapter,
		Summary:    summary,
		Characters: characters,
		KeyEvents:  keyEvents,
	}); err != nil {
		return nil, fmt.Errorf("rewrite: save summary: %w: %w", errs.ErrStoreWrite, err)
	}

	// 4. Cập nhật số từ (MarkChapterComplete là bình thường đối với các chương đã hoàn thành: thay thế số từ, slice.Contains để ngăn chặn việc xếp hàng lặp lại)
	if err := t.store.Progress.MarkChapterComplete(chapter, wordCount, hookType, dominantStrand); err != nil {
		return nil, fmt.Errorf("rewrite: update word count: %w: %w", errs.ErrStoreWrite, err)
	}

	// 5. Xả hàng đợi chờ; khi hàng đợi trống, CompleteRewrite sẽ tự động chuyển luồng quay lại ghi
	if err := t.store.Progress.CompleteRewrite(chapter); err != nil {
		return nil, fmt.Errorf("rewrite: complete rewrite: %w: %w", errs.ErrStoreWrite, err)
	}

	// 6. Checkpoint
	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(chapter), "commit",
		fmt.Sprintf("chapters/%02d.md", chapter),
	); err != nil {
		return nil, fmt.Errorf("rewrite: checkpoint commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 7. Đọc ảnh chụp nhanh tiến trình sau khi xả và trả lại như một sự thật
	mode := "rewrite"
	if progress.Flow == domain.FlowPolishing {
		mode = "polish"
	}
	latest, _ := t.store.Progress.Load()
	remaining := []int{}
	nextChapter := chapter + 1
	flow := string(domain.FlowWriting)
	if latest != nil {
		remaining = append(remaining, latest.PendingRewrites...)
		nextChapter = latest.NextChapter()
		flow = string(latest.Flow)
	}
	drained := len(remaining) == 0

	// Việc hoàn thành sẽ được xác định sau khi hàng đợi được xóa: việc gửi lại công việc không đi qua đường dẫn chính applyCompletion và việc hoàn thành chỉ có thể được kích hoạt tại đây.
	//   - Layering + Forward writing: Sử dụng mức chất lượng layeredBookComplete (yêu cầu đầu mối để bọc lại), và nhường chỗ cho kiến ​​trúc sư nếu chưa hài lòng.
	//   - Phân lớp + mở lại Rework (ReopenedFromComplete): Rework chỉ thay đổi các chương hiện có, không tăng giảm cấu trúc và dựa trên tính toàn vẹn của cấu trúc
	//     Tức là hoàn thiện lại - nếu một đầu mối nào đó bị xáo trộn do làm lại và chữ viết bị kẹt, ở cuối tập cuối cùng, nó sẽ rơi vào một vòng lặp vô tận của việc viết vượt quá giới hạn.
	//   - Non-layered: Hoàn thành khi đã lấp đầy Tổng số Chương (làm lại không tăng giảm số chương, đã đầy rồi).
	bookComplete := false
	if drained && latest != nil {
		reComplete := false
		switch {
		case latest.Layered && latest.ReopenedFromComplete:
			reComplete = t.layeredStructurallyComplete(latest)
		case latest.Layered:
			reComplete = t.layeredBookComplete(latest)
		default:
			reComplete = latest.TotalChapters > 0 && len(latest.CompletedChapters) >= latest.TotalChapters
		}
		if reComplete {
			if cerr := t.store.Progress.MarkComplete(); cerr == nil {
				bookComplete = true
				if p, _ := t.store.Progress.Load(); p != nil {
					flow = string(p.Flow)
				}
			}
		}
	}

	// Tương tự như đường dẫn chính: viết lại/đánh bóng cũng thực hiện kiểm tra cơ học và đính kèm quy tắc_violations
	violations := t.checkRules(content, wordCount)
	return json.Marshal(map[string]any{
		"chapter":         chapter,
		"rewritten":       true,
		"mode":            mode,
		"word_count":      wordCount,
		"remaining_queue": remaining,
		"queue_drained":   drained,
		"next_chapter":    nextChapter,
		"flow":            flow,
		"book_complete":   bookComplete,
		"rule_violations": violations,
	})
}

// buildSkipResult Xây dựng một kết quả trả về thực tế phù hợp với một cam kết thông thường cho "Cam kết trùng lặp đã hoàn thành chương".
// Người điều phối đưa ra các quyết định tiếp theo dựa trên điều này (công văn của nhà văn/biên tập viên/kiến trúc sư) mà không bị ảo giác khi nhận được lời nhắc về văn xuôi.
func (t *CommitChapterTool) buildSkipResult(chapter int, progress *domain.Progress) (json.RawMessage, error) {
	_, wordCount, _ := t.store.Drafts.LoadChapterContent(chapter)

	result := domain.CommitResult{
		Chapter:     chapter,
		Committed:   true,
		WordCount:   wordCount,
		NextChapter: chapter + 1,
	}

	if progress != nil && progress.Layered {
		if boundary, _ := t.store.Outline.CheckArcBoundary(chapter); boundary != nil {
			result.ArcEnd = boundary.IsArcEnd
			result.VolumeEnd = boundary.IsVolumeEnd
			result.Volume = boundary.Volume
			result.Arc = boundary.Arc
			result.NeedsExpansion = boundary.NeedsExpansion
			result.NeedsNewVolume = boundary.NeedsNewVolume
			result.NextVolume = boundary.NextVolume
			result.NextArc = boundary.NextArc
		}
		result.ReviewRequired, result.ReviewReason = domain.ShouldArcReview(result.ArcEnd, result.VolumeEnd, result.Volume, result.Arc)
	} else if progress != nil {
		result.ReviewRequired, result.ReviewReason = domain.ShouldReview(len(progress.CompletedChapters))
	}

	if progress != nil {
		if progress.Phase == domain.PhaseComplete {
			result.BookComplete = true
		}
		result.Flow = string(progress.Flow)
	}

	return json.Marshal(result)
}

// LoadCoreCharacterNameSet tải bộ tên ký tự (bao gồm cả bí danh) đã có trong character.json.
// Được sử dụng làm bộ lọc "lõi đã biết" cho cast_ledger - các ký tự cốt lõi không tạo thành danh sách phụ.
// Trả về 0 khi tải không thành công (tất cả các ký tự được nhập vào sổ cái trong quá trình hợp nhất, điều này có thể chấp nhận được).
func loadCoreCharacterNameSet(s *store.Store) map[string]bool {
	chars, err := s.Characters.Load()
	if err != nil || len(chars) == 0 {
		return nil
	}
	set := make(map[string]bool, len(chars)*2)
	for _, c := range chars {
		if c.Name != "" {
			set[c.Name] = true
		}
		for _, alias := range c.Aliases {
			if alias != "" {
				set[alias] = true
			}
		}
	}
	return set
}

// applyCompletion xác định xem cam kết này có hoàn thành cuốn sách hay không, nếu vậy, MarkComplete và trả về true.
//   - Không phân tầng: Hoàn thành sau khi viết đủ số chương đã thống nhất.
//   - Phân lớp: Kiến trúc sư rõ ràng save_foundation type=complete_book là đường dẫn chính; thêm một cái khác ở đây
//     Kết luận xác định - tự động kết thúc khi toàn bộ cuốn sách đã đáp ứng một cách khách quan các điều kiện hoàn thành (xem layeredBookComplete).
//     Ngăn chặn mô hình không cho thêm vào phần bổ sung hoặc hoàn thành ở điểm cuối, dẫn đến "người viết khỏa thân và vượt qua chương ranh giới →"
//     Đánh chặn của lính biên phòng → thử lại nhiều lần" livelock (nguyên nhân sâu xa của vụ án "Mortal Bones" ch204..347).
func (t *CommitChapterTool) applyCompletion(result *domain.CommitResult, progress *domain.Progress) bool {
	if progress == nil {
		return false
	}
	if progress.Layered {
		if t.layeredBookComplete(progress) {
			_ = t.store.Progress.MarkComplete()
			return true
		}
		return false
	}
	if progress.TotalChapters > 0 && result.NextChapter > progress.TotalChapters {
		_ = t.store.Progress.MarkComplete()
		return true
	}
	return false
}

// layeredStructurallyComplete xác định xem tiểu thuyết phân lớp có được "hoàn thành về mặt cấu trúc" hay không: hàng làm lại trống + không có cung xương nào được mở rộng
// + Tất cả các chương mở rộng đã được viết. Đây là một thực tế trạng thái cuối cùng mang tính quyết định và không chứa các phán đoán ngữ nghĩa như điềm báo/giải thích dài hạn - được sử dụng như một "phòng thủ chống lại trạng thái cuối cùng"
// Mạng lưới an toàn "Vòng lặp vô hạn" (việc làm lại và làm trống sẽ được hoàn thành tương ứng).
func (t *CommitChapterTool) layeredStructurallyComplete(progress *domain.Progress) bool {
	// 1. Hàng đợi làm lại phải được xóa
	if len(progress.PendingRewrites) > 0 {
		return false
	}
	volumes, err := t.store.Outline.LoadLayeredOutline()
	if err != nil || len(volumes) == 0 {
		return false
	}
	// 2. Không được xây dựng cốt truyện (còn nội dung ghi trong kế hoạch)
	for i := range volumes {
		for j := range volumes[i].Arcs {
			if !volumes[i].Arcs[j].IsExpanded() {
				return false
			}
		}
	}
	// 3. Tất cả các chương mở rộng phải được viết.
	expanded := len(domain.FlattenOutline(volumes))
	return expanded > 0 && len(progress.CompletedChapters) >= expanded
}

// layeredBookComplete sử dụng các dữ kiện khách quan để xác định xem cuốn sách dài được xếp lớp có thực sự được hoàn thành hay không và so sánh nó với Architect-long.md để xác định xem nó đã được hoàn thành hay chưa.
// Một danh sách các mục có thể định lượng + sự kiện cấu trúc. Ngoài việc có một cấu trúc hoàn chỉnh, nó còn đòi hỏi những điềm báo phải được đặt lại về 0 và sự hội tụ lâu dài - nếu có điều gì không hài lòng sẽ
// Nhường đường cho kiến ​​trúc sư tiếp tục mở rộng_arc/append_volume và đừng bao giờ vội kết thúc câu chuyện trước khi nó kết thúc. Bảo thủ không có la bàn
// Được đánh giá là chưa hoàn thành. Đây là đánh giá hoàn thành "mức chất lượng" đối với việc viết chuyển tiếp, chặt chẽ hơn so với LayeredStructurallyComplete.
func (t *CommitChapterTool) layeredBookComplete(progress *domain.Progress) bool {
	if !t.layeredStructurallyComplete(progress) {
		return false
	}
	// 4. Hoạt động báo trước phải được đặt lại về 0 (lời hứa đã được thực hiện)
	if active, aerr := t.store.World.LoadActiveForeshadow(); aerr != nil || len(active) > 0 {
		return false
	}
	// 5. Các đường dài hoạt động bằng la bàn phải được đóng lại (không có la bàn/các đường dài chưa được làm rõ sẽ được trả lại cho kiến ​​trúc sư quyết định)
	compass, cerr := t.store.Outline.LoadCompass()
	if cerr != nil || compass == nil || len(compass.OpenThreads) > 0 {
		return false
	}
	return true
}
