package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/store"
)

// ReadChapterTool đọc văn bản gốc của chương, cho phép Tác nhân đọc lại văn bản của chính nó và văn bản trước đó.
type ReadChapterTool struct {
	store *store.Store
}

func NewReadChapterTool(store *store.Store) *ReadChapterTool {
	return &ReadChapterTool{store: store}
}

func (t *ReadChapterTool) Name() string { return "read_chapter" }
func (t *ReadChapterTool) Description() string {
	return "Đọc văn bản gốc của chương. Đọc bản nháp cuối cùng, bản nháp hoặc trích đoạn đoạn hội thoại của nhân vật"
}
func (t *ReadChapterTool) Label() string { return "đọc chương" }

// Một công cụ đọc thuần túy có thể lên lịch đồng thời (người biên tập thường đọc nhiều chương cùng một lúc khi ôn tập).
func (t *ReadChapterTool) ReadOnly(_ json.RawMessage) bool        { return true }
func (t *ReadChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *ReadChapterTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapter", schema.Int("Số chương (bắt buộc khi đọc một chương)")),
		schema.Property("from", schema.Int("Số chương bắt đầu (được sử dụng khi đọc phạm vi)")),
		schema.Property("to", schema.Int("Số chương kết thúc (dùng khi đọc phạm vi)")),
		schema.Property("source", schema.Enum("nguồn", "final", "draft")).Required(),
		schema.Property("character", schema.String("Tên nhân vật (dùng khi trích đoạn hội thoại)")),
		schema.Property("max_runes", schema.Int("Số ký tự tối đa mỗi chương (cắt ngắn khi đọc phạm vi, mặc định 2000)")),
	)
}

func (t *ReadChapterTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter   int    `json:"chapter"`
		From      int    `json:"from"`
		To        int    `json:"to"`
		Source    string `json:"source"`
		Character string `json:"character"`
		MaxRunes  int    `json:"max_runes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	// Chế độ 1: Trích đoạn hội thoại nhân vật
	if a.Character != "" {
		chars, _ := t.store.Characters.Load()
		var aliases []string
		for _, c := range chars {
			if c.Name == a.Character {
				aliases = c.Aliases
				break
			}
		}
		var maxCompleted int
		if p, _ := t.store.Progress.Load(); p != nil {
			maxCompleted = maxCompletedChapter(p.CompletedChapters)
		}
		samples := t.store.Drafts.ExtractDialogue(a.Character, aliases, 8, maxCompleted)
		result := map[string]any{
			"character": a.Character,
			"samples":   samples,
		}
		if len(samples) == 0 {
			result["hint"] = "Hiện tại không có mẫu hội thoại cho nhân vật này. Không cần phải thử lại và chuyển thẳng sang bước tiếp theo."
		}
		return json.Marshal(result)
	}

	// Chế độ 2: Đọc phạm vi
	if a.From > 0 && a.To > 0 {
		maxRunes := a.MaxRunes
		if maxRunes <= 0 {
			maxRunes = 2000
		}
		texts, err := t.store.Drafts.LoadChapterRange(a.From, a.To, maxRunes)
		if err != nil {
			return nil, fmt.Errorf("load chapter range: %w", err)
		}
		return json.Marshal(map[string]any{
			"chapters": texts,
			"from":     a.From,
			"to":       a.To,
		})
	}

	// Chế độ 3: Đọc từng chương
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter is required")
	}

	var content string
	var err error
	switch a.Source {
	case "draft":
		content, err = t.store.Drafts.LoadDraft(a.Chapter)
	default: // final
		content, err = t.store.Drafts.LoadChapterText(a.Chapter)
		if err == nil && content == "" {
			slog.Warn("read_chapter đọc bản nháp cuối cùng và quay lại bản nháp nếu nó trống.", "module", "tool", "chapter", a.Chapter)
			content, err = t.store.Drafts.LoadDraft(a.Chapter)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read chapter %d: %w", a.Chapter, err)
	}
	if content == "" {
		return json.Marshal(map[string]any{
			"chapter": a.Chapter,
			"exists":  false,
			"hint":    "Chương này vẫn chưa được viết. Nếu bạn cần viết, vui lòng gọi Draft_chapter trước.",
		})
	}

	return json.Marshal(map[string]any{
		"chapter":    a.Chapter,
		"content":    content,
		"word_count": len([]rune(content)),
	})
}

// maxCompletedChapter Trả về số chương tối đa trong danh sách các chương đã hoàn thành.
func maxCompletedChapter(completed []int) int {
	m := 0
	for _, ch := range completed {
		if ch > m {
			m = ch
		}
	}
	return m
}
