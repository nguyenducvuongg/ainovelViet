package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

func TestSaveReviewPersistsContractAssessment(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(3, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":           3,
		"scope":             "chapter",
		"dimensions":        []map[string]any{{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Về cơ bản là giống nhau"}, {"dimension": "character", "score": 82, "verdict": "pass", "comment": "Nhân vật ổn định"}, {"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "Chậm hơn một chút"}, {"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "mạch lạc"}, {"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "Bình thường"}, {"dimension": "hook", "score": 76, "verdict": "warning", "comment": "Móc trung bình"}, {"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "Ngôn ngữ cơ bản được hình thành"}},
		"issues":            []map[string]any{},
		"contract_status":   "partial",
		"contract_misses":   []string{"Lời mời thử cửa trong không được chôn giấu rõ ràng"},
		"contract_notes":    "Đã đạt được tiến độ chính nhưng hạng mục tiến bộ thứ 2 trong hợp đồng vẫn chưa được thực hiện.",
		"verdict":           "polish",
		"summary":           "Chương này về cơ bản đã hoàn thành mục tiêu nhưng vẫn còn thiếu một số mục trong hợp đồng.",
		"affected_chapters": []int{3},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	review, err := s.World.LoadReview(3)
	if err != nil {
		t.Fatalf("LoadReview: %v", err)
	}
	if review == nil {
		t.Fatal("expected review saved, got nil")
	}
	if review.ContractStatus != "partial" {
		t.Fatalf("unexpected contract status: %q", review.ContractStatus)
	}
	if len(review.ContractMisses) != 1 || review.ContractMisses[0] != "Lời mời thử cửa trong không được chôn giấu rõ ràng" {
		t.Fatalf("unexpected contract misses: %+v", review.ContractMisses)
	}
	if review.Dimension("aesthetic") == nil {
		t.Fatalf("expected aesthetic dimension persisted, got %+v", review.Dimensions)
	}
}

func TestSaveReviewRejectsMissingDimensions(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(3, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":    3,
		"scope":      "chapter",
		"dimensions": []map[string]any{{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Về cơ bản là giống nhau"}},
		"issues":     []map[string]any{},
		"verdict":    "accept",
		"summary":    "ok",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "dimensions must contain exactly") {
		t.Fatalf("expected dimensions validation error, got %v", err)
	}
}

func TestSaveReviewRejectsDimensionWithoutComment(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(3, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "comment": "Về cơ bản là giống nhau"},
			{"dimension": "character", "score": 82, "comment": "Nhân vật ổn định"},
			{"dimension": "pacing", "score": 78},
			{"dimension": "continuity", "score": 84, "comment": "mạch lạc"},
			{"dimension": "foreshadow", "score": 80, "comment": "Bình thường"},
			{"dimension": "hook", "score": 76, "comment": "Móc trung bình"},
			{"dimension": "aesthetic", "score": 81, "comment": "Ngôn ngữ cơ bản được hình thành"},
		},
		"issues":  []map[string]any{},
		"verdict": "accept",
		"summary": "ok",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "dimension comment is required: pacing") {
		t.Fatalf("expected dimension comment validation error, got %v", err)
	}
}

func TestSaveReviewRejectsUnfinishedAffectedChapter(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 80); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	for ch := 1; ch <= 58; ch++ {
		if err := s.Progress.MarkChapterComplete(ch, 3000, "", ""); err != nil {
			t.Fatalf("MarkChapterComplete(%d): %v", ch, err)
		}
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 58,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "comment": "Về cơ bản là giống nhau"},
			{"dimension": "character", "score": 82, "comment": "Nhân vật ổn định"},
			{"dimension": "pacing", "score": 58, "comment": "Nhịp điệu cần viết lại"},
			{"dimension": "continuity", "score": 84, "comment": "mạch lạc"},
			{"dimension": "foreshadow", "score": 80, "comment": "Bình thường"},
			{"dimension": "hook", "score": 76, "comment": "Móc trung bình"},
			{"dimension": "aesthetic", "score": 81, "comment": "Ngôn ngữ cơ bản được hình thành"},
		},
		"issues":            []map[string]any{},
		"contract_status":   "partial",
		"verdict":           "polish",
		"summary":           "Chương 58 cần phải trau chuốt, những chương chưa hoàn thành không thể thêm vào hàng đợi.",
		"affected_chapters": []int{65},
		"contract_misses":   []string{"Nhịp điệu vượt quá trách nhiệm của chương này"},
		"contract_notes":    "Chỉ những chương đã hoàn thành mới được xử lý.",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "đang chờ xử lý_rewrites chỉ có thể chứa các chương đã hoàn thành") {
		t.Fatalf("expected unfinished affected chapter rejection, got %v", err)
	}
	review, err := s.World.LoadReview(58)
	if err != nil {
		t.Fatalf("LoadReview: %v", err)
	}
	if review != nil {
		t.Fatalf("review should not be saved when pending rewrite validation fails: %+v", review)
	}
	p, _ := s.Progress.Load()
	if p.Flow != domain.FlowWriting && p.Flow != "" {
		t.Fatalf("flow should not enter rewrite/polish, got %s", p.Flow)
	}
	if len(p.PendingRewrites) != 0 {
		t.Fatalf("pending_rewrites should remain empty, got %v", p.PendingRewrites)
	}
}

// TestSaveReviewDerivesVerdictFromScore Xác minh: phán quyết được xác định dựa trên điểm số do mô hình đưa ra
// Các kết quả không nhất quán (chẳng hạn như điểm=85 nhưng điền vào cảnh báo) không còn báo lỗi mà được ghi đè thành giá trị chính xác (đạt).
// Vấn đề chống hồi quy: Điểm số mô hình/phán quyết yếu khiến cho save_review liên tục thất bại.
func TestSaveReviewDerivesVerdictFromScore(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(3, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "nhất quán"},
			{"dimension": "character", "score": 82, "comment": "Ổn định"}, // bỏ qua bản án
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "Chậm hơn một chút"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "mạch lạc"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "Bình thường"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "Móc trung bình"},
			{"dimension": "aesthetic", "score": 85, "verdict": "warning", "comment": "ngôn ngữ được thành lập"}, // Không nhất quán: 85 điền vào cảnh báo
		},
		"issues":  []map[string]any{},
		"verdict": "accept",
		"summary": "ok",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute should succeed (verdict auto-derived), got %v", err)
	}

	review, err := s.World.LoadReview(3)
	if err != nil || review == nil {
		t.Fatalf("LoadReview: %v", err)
	}
	// 85 → vượt qua (ghi đè cảnh báo do mô hình đưa ra); 82 bỏ qua → vượt qua.
	if d := review.Dimension("aesthetic"); d == nil || d.Verdict != "pass" {
		t.Fatalf("aesthetic verdict should be derived to pass, got %+v", d)
	}
	if d := review.Dimension("character"); d == nil || d.Verdict != "pass" {
		t.Fatalf("character verdict should be derived to pass, got %+v", d)
	}
}

func TestSaveReviewRejectsMissingAffectedChaptersForRewrite(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Về cơ bản là giống nhau"},
			{"dimension": "character", "score": 82, "verdict": "pass", "comment": "Nhân vật ổn định"},
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "Chậm hơn một chút"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "mạch lạc"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "Bình thường"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "Móc trung bình"},
			{"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "Ngôn ngữ cơ bản được hình thành"},
		},
		"issues":  []map[string]any{},
		"verdict": "rewrite",
		"summary": "Cần phải viết lại",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "affected_chapters is required") {
		t.Fatalf("expected affected_chapters validation error, got %v", err)
	}
}

func TestSaveReviewRejectsIssueWithoutEvidence(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Về cơ bản là giống nhau"},
			{"dimension": "character", "score": 82, "verdict": "pass", "comment": "Nhân vật ổn định"},
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "Chậm hơn một chút"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "mạch lạc"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "Bình thường"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "Móc trung bình"},
			{"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "Ngôn ngữ cơ bản được hình thành"},
		},
		"issues": []map[string]any{
			{"type": "hook", "severity": "warning", "description": "Cái móc cuối chương yếu quá"},
		},
		"verdict":           "polish",
		"summary":           "Cần tăng cường móc.",
		"affected_chapters": []int{3},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "issue evidence is required") {
		t.Fatalf("expected issue evidence validation error, got %v", err)
	}
}
