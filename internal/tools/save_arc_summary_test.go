package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

func TestSaveArcSummaryPersistsStyleRulesDialogueObjects(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveArcSummaryTool(s)
	args, err := json.Marshal(map[string]any{
		"volume":     1,
		"arc":        2,
		"title":      "Vào núi",
		"summary":    "Nhân vật chính hoàn thành thử thách trên núi và xác nhận phương hướng truy đuổi tiếp theo.",
		"key_events": []string{"Vượt qua thử nghiệm", "Khám phá manh mối trong vụ án cũ"},
		"character_snapshots": []map[string]any{
			{"name": "Thẩm Viên", "status": "tồn tại", "motivation": "theo đuổi vụ án cũ"},
		},
		"style_rules": map[string]any{
			"prose": []string{"Mô tả môi trường ưu tiên chạm và ngửi", "Những cảnh hành động được nâng cao bằng những câu thoại ngắn", "Mô tả tâm lý không giải thích được kết luận"},
			"dialogue": []map[string]any{
				{"name": "Thẩm Viên", "rules": []string{"Đối thoại tối giản", "Sử dụng ít câu hỏi hơn"}},
			},
			"taboos": []string{"Tránh độc thoại dài dòng ở cuối chương"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	rules, err := s.World.LoadStyleRules()
	if err != nil {
		t.Fatalf("LoadStyleRules: %v", err)
	}
	if rules == nil || len(rules.Dialogue) != 1 {
		t.Fatalf("expected one dialogue rule, got %+v", rules)
	}
	if rules.Dialogue[0].Name != "Thẩm Viên" || len(rules.Dialogue[0].Rules) != 2 {
		t.Fatalf("unexpected dialogue rule: %+v", rules.Dialogue[0])
	}
}

func TestSaveArcSummaryRejectsDialogueStringArray(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveArcSummaryTool(s)
	args, err := json.Marshal(map[string]any{
		"volume":              1,
		"arc":                 2,
		"title":               "Vào núi",
		"summary":             "Nhân vật chính hoàn thành thử thách trên núi và xác nhận phương hướng truy đuổi tiếp theo.",
		"key_events":          []string{"Vượt qua thử nghiệm"},
		"character_snapshots": []map[string]any{},
		"style_rules": map[string]any{
			"prose":    []string{"Mô tả môi trường ưu tiên chạm và ngửi"},
			"dialogue": []string{"Lời thoại của Shen Yuan tối giản"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "style_rules.dialogue") {
		t.Fatalf("expected style_rules.dialogue validation error, got %v", err)
	}
}
