package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

func TestReadChapterFinal(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Drafts.SaveFinalChapter(1, "Văn bản cuối cùng của Chương Một."); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}

	tool := NewReadChapterTool(store)
	args, _ := json.Marshal(map[string]any{"chapter": 1, "source": "final"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Chapter   int    `json:"chapter"`
		Content   string `json:"content"`
		WordCount int    `json:"word_count"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Content == "" {
		t.Fatal("expected non-empty content")
	}
	if payload.WordCount == 0 {
		t.Fatal("expected non-zero word count")
	}
}

func TestReadChapterDraft(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Drafts.SaveDraft(3, "Dự thảo nội dung Chương 3."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewReadChapterTool(store)
	args, _ := json.Marshal(map[string]any{"chapter": 3, "source": "draft"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Content == "" {
		t.Fatal("expected draft content")
	}
}

func TestReadChapterDialogue(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Characters.Save([]domain.Character{
		{Name: "Trương Tam", Aliases: []string{"Lão Trương"}},
	}); err != nil {
		t.Fatalf("SaveCharacters: %v", err)
	}
	if err := store.Drafts.SaveFinalChapter(1, "Trương San đứng dậy. \u201cTôi không đồng ý với kế hoạch này,\u201d Zhang San lạnh lùng nói."); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}

	tool := NewReadChapterTool(store)
	args, _ := json.Marshal(map[string]any{"source": "final", "character": "Trương Tam"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Character string   `json:"character"`
		Samples   []string `json:"samples"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Character != "Trương Tam" {
		t.Fatalf("nhân vật được mong đợi Zhang San, có %s", payload.Character)
	}
}

func TestReadChapterRange(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if err := store.Drafts.SaveFinalChapter(i, "Đây là một phần nội dung văn bản."); err != nil {
			t.Fatalf("SaveFinalChapter(%d): %v", i, err)
		}
	}

	tool := NewReadChapterTool(store)
	args, _ := json.Marshal(map[string]any{"from": 1, "to": 3, "source": "final"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Chapters map[string]string `json:"chapters"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload.Chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(payload.Chapters))
	}
}

func TestDraftChapterWrite(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewDraftChapterTool(store)
	args, _ := json.Marshal(map[string]any{
		"chapter": 1,
		"content": "Đây là toàn bộ văn bản của chương, được viết trong một lần.",
		"mode":    "write",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Written   bool `json:"written"`
		WordCount int  `json:"word_count"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !payload.Written {
		t.Fatal("expected written=true")
	}
	if payload.WordCount == 0 {
		t.Fatal("expected non-zero word count")
	}

	// Xác minh rằng nó có thể được đọc lại
	content, err := store.Drafts.LoadDraft(1)
	if err != nil {
		t.Fatalf("LoadDraft: %v", err)
	}
	if content == "" {
		t.Fatal("expected non-empty draft")
	}
	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if progress.InProgressChapter != 1 {
		t.Fatalf("expected in-progress chapter 1, got %d", progress.InProgressChapter)
	}
	if progress.Phase != domain.PhaseWriting {
		t.Fatalf("expected phase writing, got %s", progress.Phase)
	}
}

func TestDraftChapterAppend(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := store.Drafts.SaveDraft(2, "Nửa đầu."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewDraftChapterTool(store)
	args, _ := json.Marshal(map[string]any{
		"chapter": 2,
		"content": "nửa sau.",
		"mode":    "append",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Mode      string `json:"mode"`
		WordCount int    `json:"word_count"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Mode != "append" {
		t.Fatalf("expected mode=append, got %s", payload.Mode)
	}

	content, _ := store.Drafts.LoadDraft(2)
	if content == "" || content == "Nửa đầu." {
		t.Fatal("expected appended content")
	}
}

func TestReadChapterMissingReturnsJSON(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewReadChapterTool(store)
	args, _ := json.Marshal(map[string]any{"chapter": 1, "source": "final"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("expected no error for missing chapter, got: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if parsed["exists"] != false {
		t.Fatal("expected exists=false")
	}
}

func TestPlanChapterMarksInProgress(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewPlanChapterTool(store)
	args, _ := json.Marshal(map[string]any{
		"chapter":  1,
		"title":    "sự khởi đầu",
		"goal":     "Thiết lập một tình huống",
		"conflict": "Nợ chồng chất",
		"hook":     "khám phá manh mối",
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if progress.Phase != domain.PhaseWriting {
		t.Fatalf("expected phase writing, got %s", progress.Phase)
	}
	if progress.InProgressChapter != 1 {
		t.Fatalf("expected in-progress chapter 1, got %d", progress.InProgressChapter)
	}
}

func TestDraftChapterRejectsCompleted(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	_ = s.Drafts.SaveDraft(1, "Văn bản chương 1")
	_ = s.Progress.StartChapter(1)
	_ = s.Progress.MarkChapterComplete(1, 3000, "", "")

	tool := NewDraftChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter": 1,
		"content": "Đang cố gắng ghi đè một chương đã gửi",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("expected soft rejection, got error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload["skipped"] != true {
		t.Fatalf("expected skipped=true, got %+v", payload)
	}
}
