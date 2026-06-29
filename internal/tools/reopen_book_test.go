package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// CompletedBook xây dựng một cuốn tiểu thuyết N-chương đã hoàn thành (giai đoạn=hoàn thành, CompletedChapters=1..n).
func completedBook(t *testing.T, n int) *store.Store {
	t.Helper()
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", n); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	for ch := 1; ch <= n; ch++ {
		if err := s.Progress.MarkChapterComplete(ch, 100, "", ""); err != nil {
			t.Fatalf("MarkChapterComplete(%d): %v", ch, err)
		}
	}
	if err := s.Progress.MarkComplete(); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	return s
}

func TestReopenBookReopensCompletedBook(t *testing.T) {
	s := completedBook(t, 3)
	tool := NewReopenBookTool(s)

	args, _ := json.Marshal(map[string]any{"chapters": []int{3, 1}, "reason": "Làm sạch các ký tự đặc biệt"})
	raw, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload["reopened"] != true || payload["phase"] != string(domain.PhaseWriting) {
		t.Fatalf("unexpected payload: %v", payload)
	}

	p, _ := s.Progress.Load()
	if p.Phase != domain.PhaseWriting {
		t.Errorf("phase = %s, want writing", p.Phase)
	}
	if p.Flow != domain.FlowRewriting {
		t.Errorf("flow = %s, want rewriting", p.Flow)
	}
	if len(p.PendingRewrites) != 2 || p.PendingRewrites[0] != 3 || p.PendingRewrites[1] != 1 {
		t.Errorf("PendingRewrites = %v, muốn [3 1] (enqueue as is)", p.PendingRewrites)
	}

	if cp := s.Checkpoints.LatestByStep(domain.GlobalScope(), "reopen"); cp == nil {
		t.Error("expected a 'reopen' checkpoint")
	}
}

func TestReopenBookRejectsNonCompleteBook(t *testing.T) {
	// Sách đang viết (chưa hoàn thành) không thể mở lại được
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 5); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(1, 100, "", ""); err != nil { // phase→writing
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	tool := NewReopenBookTool(s)
	args, _ := json.Marshal(map[string]any{"chapters": []int{1}})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected reopen to be rejected when phase != complete")
	}
}

func TestReopenBookRejectsUnwrittenChapters(t *testing.T) {
	s := completedBook(t, 3)
	tool := NewReopenBookTool(s)

	// Chương 5 không tồn tại → Bị từ chối (đó là phần tiếp theo/ngoài giới hạn và cần được điều chỉnh độ dài)
	args, _ := json.Marshal(map[string]any{"chapters": []int{2, 5}})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected reopen to be rejected for unwritten chapter")
	}
	// chương trống → từ chối
	args, _ = json.Marshal(map[string]any{"chapters": []int{}})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected reopen to be rejected for empty chapters")
	}
}
