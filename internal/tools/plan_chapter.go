package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
)

// PlanChapterTool lưu ý tưởng chương và Tác nhân xác định mức độ chi tiết của kế hoạch một cách độc lập.
type PlanChapterTool struct {
	store *store.Store
}

func NewPlanChapterTool(store *store.Store) *PlanChapterTool {
	return &PlanChapterTool{store: store}
}

func (t *PlanChapterTool) Name() string { return "plan_chapter" }
func (t *PlanChapterTool) Description() string {
	return "Lưu ý tưởng viết chương. Tác nhân quyết định mức độ chi tiết của kế hoạch một cách độc lập và không bắt buộc phải phân chia kịch bản."
}
func (t *PlanChapterTool) Label() string { return "chương kế hoạch" }

// Viết công cụ để vô hiệu hóa đồng thời.
func (t *PlanChapterTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *PlanChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *PlanChapterTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapter", schema.Int("số chương")).Required(),
		schema.Property("title", schema.String("Tiêu đề chương")).Required(),
		schema.Property("goal", schema.String("Mục tiêu của chương này")).Required(),
		schema.Property("conflict", schema.String("xung đột cốt lõi")).Required(),
		schema.Property("hook", schema.String("móc cuối chương")).Required(),
		schema.Property("emotion_arc", schema.String("đường cong tình cảm")),
		schema.Property("notes", schema.String("Ghi chú miễn phí (bất cứ điều gì bạn cảm thấy cần nhớ khi viết)")),
		schema.Property("required_beats", schema.Array("Các mục nâng cao phải được hoàn thành trong chương này", schema.String(""))),
		schema.Property("forbidden_moves", schema.Array("Chương này nói rõ rằng sự thăng tiến không thể xảy ra", schema.String(""))),
		schema.Property("continuity_checks", schema.Array("Các điểm liên tục cần kiểm tra đặc biệt trong chương này", schema.String(""))),
		schema.Property("evaluation_focus", schema.Array("Các mục kiểm tra phím soạn thảo", schema.String(""))),
		schema.Property("emotion_target", schema.String("Tùy chọn: Những cảm xúc chính mà bạn muốn người đọc cảm nhận được trong chương này")),
		schema.Property("payoff_points", schema.Array("Tùy chọn: Điểm cốt truyện hoặc điểm thực hiện mà bạn muốn phản hồi trong chương chính", schema.String(""))),
		schema.Property("hook_goal", schema.String("Tùy chọn: Mục tiêu đọc theo hướng hy vọng hoặc mục tiêu hồi hộp ở cuối chương")),
	)
}

func (t *PlanChapterTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	plan, err := decodeChapterPlanArgs(args)
	if err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if plan.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if t.store.Progress.IsChapterCompleted(plan.Chapter) {
		return json.Marshal(map[string]any{
			"chapter":   plan.Chapter,
			"skipped":   true,
			"completed": true,
			"reason":    fmt.Sprintf("Chương %d đã được gửi và không thể lên lịch lại.", plan.Chapter),
		})
	}
	if err := t.store.Progress.ValidateChapterWork(plan.Chapter); err != nil {
		return nil, err
	}
	if err := EnsureChapterExpanded(t.store, plan.Chapter); err != nil {
		return nil, err
	}

	if err := t.store.Drafts.SaveChapterPlan(plan); err != nil {
		return nil, fmt.Errorf("save chapter plan: %w", err)
	}
	if err := t.store.Progress.StartChapter(plan.Chapter); err != nil {
		return nil, fmt.Errorf("mark chapter in progress: %w", err)
	}

	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(plan.Chapter), "plan",
		fmt.Sprintf("drafts/%02d.plan.json", plan.Chapter),
	); err != nil {
		return nil, fmt.Errorf("checkpoint chapter plan: %w", err)
	}

	return json.Marshal(map[string]any{
		"planned":   true,
		"chapter":   plan.Chapter,
		"next_step": "Gọi ngay Draft_chapter(chapter=số chương này, content=complete text string) để viết văn bản, không lập kế hoạch lặp lại cùng một chương",
	})
}

func decodeChapterPlanArgs(args json.RawMessage) (domain.ChapterPlan, error) {
	var a struct {
		Chapter          int      `json:"chapter"`
		Title            string   `json:"title"`
		Goal             string   `json:"goal"`
		Conflict         string   `json:"conflict"`
		Hook             string   `json:"hook"`
		EmotionArc       string   `json:"emotion_arc"`
		Notes            string   `json:"notes"`
		RequiredBeats    []string `json:"required_beats"`
		ForbiddenMoves   []string `json:"forbidden_moves"`
		ContinuityChecks []string `json:"continuity_checks"`
		EvaluationFocus  []string `json:"evaluation_focus"`
		EmotionTarget    string   `json:"emotion_target"`
		PayoffPoints     []string `json:"payoff_points"`
		HookGoal         string   `json:"hook_goal"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return domain.ChapterPlan{}, err
	}

	return domain.ChapterPlan{
		Chapter:    a.Chapter,
		Title:      a.Title,
		Goal:       a.Goal,
		Conflict:   a.Conflict,
		Hook:       a.Hook,
		EmotionArc: a.EmotionArc,
		Notes:      a.Notes,
		Contract: domain.ChapterContract{
			RequiredBeats:    a.RequiredBeats,
			ForbiddenMoves:   a.ForbiddenMoves,
			ContinuityChecks: a.ContinuityChecks,
			EvaluationFocus:  a.EvaluationFocus,
			EmotionTarget:    a.EmotionTarget,
			PayoffPoints:     a.PayoffPoints,
			HookGoal:         a.HookGoal,
		},
	}, nil
}
