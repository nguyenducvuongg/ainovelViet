package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestCommitChapterSchemaDescribesFeedbackAsObject(t *testing.T) {
	tool := NewCommitChapterTool(store.NewStore(t.TempDir()))
	schema := tool.Schema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %#v", schema["properties"])
	}
	feedback, ok := props["feedback"].(map[string]any)
	if !ok {
		t.Fatalf("feedback schema missing: %#v", props["feedback"])
	}
	desc, _ := feedback["description"].(string)
	if !strings.Contains(desc, "đối tượng JSON") || !strings.Contains(desc, "JSON được xâu chuỗi") {
		t.Fatalf("feedback description should warn against stringified JSON, got %q", desc)
	}
	if got := feedback["type"]; got != "object" {
		t.Fatalf("feedback type = %v, want object", got)
	}
}

func TestCommitChapterRejectsNonPendingRewrite(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := store.Progress.MarkChapterComplete(2, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	if err := store.Progress.SetPendingRewrites([]int{2}, "viết lại bài kiểm tra"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := store.Progress.SetFlow(domain.FlowRewriting); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}
	if err := store.Drafts.SaveDraft(3, "Đây là văn bản của chương sai."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(store)
	args, err := json.Marshal(map[string]any{
		"chapter":         3,
		"summary":         "Gửi sai",
		"characters":      []string{"nhân vật chính"},
		"key_events":      []string{"Gửi do nhầm lẫn"},
		"timeline_events": []any{},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected commit to be rejected during rewrite flow")
	}

	if _, err := os.Stat(dir + "/chapters/03.md"); !os.IsNotExist(err) {
		t.Fatalf("chapter should not be persisted, stat err=%v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if len(progress.CompletedChapters) != 1 || progress.CompletedChapters[0] != 2 {
		t.Fatalf("completed chapters should only contain original chapter 2, got %v", progress.CompletedChapters)
	}
	if progress.CurrentChapter != 3 {
		t.Fatalf("current chapter should not advance beyond original progress, got %d", progress.CurrentChapter)
	}
}

func TestCommitChapterAllowsPendingRewrite(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := store.Progress.MarkChapterComplete(2, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	if err := store.Progress.SetPendingRewrites([]int{2}, "viết lại bài kiểm tra"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := store.Progress.SetFlow(domain.FlowRewriting); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}
	if err := store.Drafts.SaveDraft(2, "Đây là văn bản của chương chính xác được viết lại."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(store)
	args, err := json.Marshal(map[string]any{
		"chapter":         2,
		"summary":         "Gửi chính xác",
		"characters":      []string{"nhân vật chính"},
		"key_events":      []string{"viết lại hoàn chỉnh"},
		"timeline_events": []any{},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := os.Stat(dir + "/chapters/02.md"); err != nil {
		t.Fatalf("chapter should be persisted: %v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if len(progress.CompletedChapters) != 1 || progress.CompletedChapters[0] != 2 {
		t.Fatalf("unexpected completed chapters: %v", progress.CompletedChapters)
	}
	pending, err := store.Signals.LoadPendingCommit()
	if err != nil {
		t.Fatalf("LoadPendingCommit: %v", err)
	}
	if pending != nil {
		t.Fatalf("expected pending commit cleared, got %+v", pending)
	}
}

// TestCommitChapterUpdatesCastLedger Xác minh: commit_chapter tích lũy các ký tự của chương này thành cast_ledger,
// Brief_role do cast_intros cung cấp sẽ được sử dụng và các vai trò cốt lõi trong character.json không được nhập vào sổ cái.
func TestCommitChapterUpdatesCastLedger(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	// Đặt các tệp ký tự cốt lõi (những tệp này không được đưa vào cast_ledger)
	if err := s.Characters.Save([]domain.Character{
		{Name: "Lâm Mạch", Role: "nhân vật chính", Tier: "core"},
		{Name: "Lý Thanh Nham", Role: "gia sư", Tier: "important"},
	}); err != nil {
		t.Fatalf("Save core characters: %v", err)
	}
	if err := s.Drafts.SaveDraft(1, "Trong nội dung chính của chương đầu tiên, Lin Mo gặp chủ quán trọ Lao Chu và cậu bé Ayun."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    1,
		"summary":    "Nhà trọ nhận phòng Lin Mo",
		"characters": []string{"Lâm Mạch", "Lý Thanh Nham", "Lão Châu", "Ayun"},
		"key_events": []string{"Đăng ký vào"},
		"cast_intros": []any{
			map[string]any{"name": "Lão Châu", "brief_role": "chủ quán trọ"},
			map[string]any{"name": "Ayun", "brief_role": "Cậu bé quán trọ"},
		},
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	entries, err := s.Cast.Load()
	if err != nil {
		t.Fatalf("Cast.Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("dự kiến ​​có 2 mục sổ cái (Lào Châu/Ayun), có %d: %+v", len(entries), entries)
	}
	byName := map[string]domain.CastEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if e, ok := byName["Lão Châu"]; !ok || e.BriefRole != "chủ quán trọ" || e.FirstSeenChapter != 1 {
		t.Errorf("Lão Chu nhập sai: %+v", e)
	}
	if e, ok := byName["Ayun"]; !ok || e.BriefRole != "Cậu bé quán trọ" || e.AppearanceCount != 1 {
		t.Errorf("Ayun nhập sai: %+v", e)
	}
	if _, ok := byName["Lâm Mạch"]; ok {
		t.Errorf("Nhân vật cốt lõi Lin Mo không nên vào sổ cái")
	}
	if _, ok := byName["Lý Thanh Nham"]; ok {
		t.Errorf("Nhân vật cốt lõi Li Qingyan không nên được đưa vào sổ cái")
	}
}

// Xác minh TestCommitChapterRejectsPolishWithoutDraftChange: Sau khi chương hoàn thành sẽ được đưa vào hàng đợi đánh bóng/viết lại,
// Nếu người viết bỏ qua Draft_chapter và cam kết trực tiếp (nội dung của Draft và Chapter hoàn toàn giống nhau),
// commit_chapter phải bị từ chối, buộc người viết phải gọi Draft_chapter để viết phiên bản mới trước.
// TestCommitChapterNonLayeredRecompletesAfterRework xác minh rằng các sách không có lớp được mở lại và làm lại sau khi hoàn thành.
// Sau khi thay đổi cam kết chương và hàng đợi được rút hết, nó có thể tự động quay lại hoàn thành (một nhánh không phân cấp được hoàn thành sau khi lấp đầy cống).
func TestCommitChapterNonLayeredRecompletesAfterRework(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 2); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Hai chương đã được viết và hoàn thành. Chương 2 có bản nháp/chương sẵn sàng để làm lại và nộp.
	ch2 := "Văn bản gốc của Chương 2 được sử dụng để mô phỏng bản thảo cuối cùng được trình."
	if err := s.Drafts.SaveDraft(2, ch2); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(2, ch2); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(1, 100, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete(1): %v", err)
	}
	if err := s.Progress.MarkChapterComplete(2, len([]rune(ch2)), "", ""); err != nil {
		t.Fatalf("MarkChapterComplete(2): %v", err)
	}
	if err := s.Progress.MarkComplete(); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}

	// mở lại Chương 2 → quay lại viết, PendingRewrites=[2], flow=rewriting
	if err := s.Progress.Reopen([]int{2}, "Làm lại"); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	// Làm lại và nộp (bản dự thảo phải khác với bản dự thảo cuối cùng trước khi phát hành)
	if err := s.Drafts.SaveDraft(2, ch2+"\n\n làm lại và thêm đoạn văn mới."); err != nil {
		t.Fatalf("SaveDraft (reworked): %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    2,
		"summary":    "Tóm tắt sau khi làm lại",
		"characters": []string{"nhân vật chính"},
		"key_events": []string{"dọn dẹp"},
	})
	raw, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute rework commit: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload["book_complete"] != true {
		t.Errorf("book_complete = %v, want true", payload["book_complete"])
	}

	p, _ := s.Progress.Load()
	if p.Phase != domain.PhaseComplete {
		t.Errorf("giai đoạn = %s, muốn hoàn thành (sẽ tự động kết thúc lại)", p.Phase)
	}
	if len(p.PendingRewrites) != 0 {
		t.Errorf("PendingRewrites = %v, want empty", p.PendingRewrites)
	}
}

// TestCommitChapterLayeredReopenRecompletesDespiteOpenThread Đóng xác minh: mở lại sách nhiều lớp
// Sau khi làm lại, ngay cả khi la bàn vẫn còn những dòng dài chưa hoàn thành (việc làm lại có thể làm phiền nó), nó sẽ được hoàn thiện lại dưới dạng "hoàn chỉnh về mặt cấu trúc" sau khi làm trống -
// Đừng mắc kẹt trong việc viết và tránh vòng lặp vô tận của việc vượt qua ranh giới và tiếp tục viết ở cuối tập cuối cùng (§6.5 / known_outline_exhaustion family).
// Bằng chứng phản bác: Nếu đường dẫn mở lại vẫn sử dụng layeredBookComplete ở mức chất lượng, chuỗi mở trong ví dụ này sẽ trả về sai.
// book_complete là sai và kiểm tra thất bại.
func TestCommitChapterLayeredReopenRecompletesDespiteOpenThread(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Hai chương trong một tập và một vòng cung, tất cả đều được mở ra
	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "Tập 1", "theme": "chủ đề",
			"arcs": []map[string]any{{
				"index": 1, "title": "cung một", "goal": "Mục tiêu",
				"chapters": []map[string]any{
					{"title": "Chương 1", "core_event": "tăng lên", "hook": "Tiếp tục"},
					{"title": "Chương tiếp theo", "core_event": "kế thừa", "hook": "kết thúc"},
				},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}

	// Hai chương được viết và hoàn thành.
	ch2 := "Văn bản gốc của Chương 2 và mô phỏng đã được gửi dưới dạng bản thảo cuối cùng."
	for ch, body := range map[int]string{1: "Văn bản chương 1.", 2: ch2} {
		if err := s.Drafts.SaveDraft(ch, body); err != nil {
			t.Fatalf("SaveDraft %d: %v", ch, err)
		}
		if err := s.Drafts.SaveFinalChapter(ch, body); err != nil {
			t.Fatalf("SaveFinalChapter %d: %v", ch, err)
		}
		if err := s.Progress.MarkChapterComplete(ch, len([]rune(body)), "", ""); err != nil {
			t.Fatalf("MarkChapterComplete %d: %v", ch, err)
		}
	}
	if err := s.Progress.MarkComplete(); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}

	// Mô phỏng “làm lại rối dòng dài”: la bàn vẫn còn đề mở dở
	if err := s.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "Nhân vật chính trở về nhà", OpenThreads: []string{"Kẻ thù cũ chưa bị tiêu diệt"}}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}

	// mở lại Chương 2 → Gửi lại bản làm lại (bản nháp phải khác với bản nháp cuối cùng trước khi được phát hành)
	if err := s.Progress.Reopen([]int{2}, "Làm lại"); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := s.Drafts.SaveDraft(2, ch2+"\n\n làm lại và thêm đoạn văn mới."); err != nil {
		t.Fatalf("SaveDraft reworked: %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter": 2, "summary": "Tóm tắt làm lại", "characters": []string{"nhân vật chính"}, "key_events": []string{"dọn dẹp"},
	})
	raw, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute rework commit: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if bc, _ := out["book_complete"].(bool); !bc {
		t.Error("mở lại Sau khi làm lại và làm trống, cần hoàn thành lại hoàn toàn theo cấu trúc (ngay cả khi dòng dài hạn chưa bị đóng)")
	}
	p, _ := s.Progress.Load()
	if p.Phase != domain.PhaseComplete {
		t.Errorf("phase = %s, want complete", p.Phase)
	}
	if p.ReopenedFromComplete {
		t.Error("ReopenedFromComplete phải được xóa sau khi hoàn thành lại")
	}
}

func TestCommitChapterRejectsPolishWithoutDraftChange(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Mô phỏng Chương 2 đã hoàn thành bình thường: bản nháp có nội dung giống chương.
	original := "Nội dung văn bản gốc của Chương 2 được sử dụng để mô phỏng bản thảo cuối cùng được trình."
	if err := s.Drafts.SaveDraft(2, original); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(2, original); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(2, len([]rune(original)), "mystery", "quest"); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	// Nhập hàng đánh bóng: Flow=Polishing, PendingRewrites=[2]
	if err := s.Progress.SetPendingRewrites([]int{2}, "Kiểm tra đánh bóng"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := s.Progress.SetFlow(domain.FlowPolishing); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}

	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    2,
		"summary":    "Giả vờ đánh bóng nó",
		"characters": []string{"nhân vật chính"},
		"key_events": []string{"Không có thay đổi"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected commit to be rejected when drafts equals final content")
	}

	// Viết một bản nháp khác → nên vượt qua
	polished := original + "\n\n thêm đoạn sau khi đánh bóng."
	if err := s.Drafts.SaveDraft(2, polished); err != nil {
		t.Fatalf("SaveDraft (polished): %v", err)
	}
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute after real polish: %v", err)
	}
}

// TestCommitChapterLayeredRejectsOutOfRangeChapter xác minh rằng ở chế độ phân lớp,
// Các cam kết có số chương vượt quá layered_outline phải thất bại nặng nề thay vì được phát hành cùng với slog.Warn.
// Đây chính là chiếc phanh vật lý ngăn cản “người viết khỏa thân chạy trốn sau một phán đoán sai lầm” (Trường hợp 204..347 của “Xương phàm”).
func TestCommitChapterLayeredRejectsOutOfRangeChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Tạo một layered_outline chỉ có 1 tập, 1 cung và 1 chương
	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "Tập 1", "theme": "chủ đề",
			"arcs": []map[string]any{{
				"index": 1, "title": "cung một", "goal": "Mục tiêu",
				"chapters": []map[string]any{
					{"title": "Chương 1", "core_event": "tăng lên", "hook": "Tiếp tục"},
				},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	// Cam kết ngoài giới hạn chương 2 phải thất bại nặng nề
	if err := s.Drafts.SaveDraft(2, "Văn bản chương xuyên biên giới phải được dừng lại."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    2,
		"summary":    "Chương ngoài giới hạn",
		"characters": []string{"nhân vật chính"},
		"key_events": []string{"không nên được phép"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected commit to fail when chapter out of layered outline range")
	}

	// Không nên đặt các tệp chương trên đĩa và Tiến trình không được nâng cao.
	if _, statErr := os.Stat(dir + "/chapters/02.md"); !os.IsNotExist(statErr) {
		t.Fatalf("chapter 2 should not be persisted, stat err=%v", statErr)
	}
	progress, _ := s.Progress.Load()
	if len(progress.CompletedChapters) != 0 {
		t.Fatalf("CompletedChapters should stay empty, got %v", progress.CompletedChapters)
	}
}

// TestCommitChapterLayeredAutoCompletesWhenDone xác minh việc hoàn thành xác định chế độ phân lớp:
// Phác thảo được phát triển và viết đầy đủ + không có khung xương + không làm lại + không có điềm báo hoạt động + khi đóng đường dài của la bàn,
// Cam kết của chương trước sẽ tự động đẩy Phase=Complete mà không cần dựa vào kiến ​​trúc sư để chủ động điều chỉnh Complete_book.
// Đây là bản sửa lỗi cho livelock được giới thiệu sau khi xóa hoàn thành tự động phân lớp trong 9bf26a5 (mô hình ở cuối tập cuối không thêm
// Cũng chưa trọn vẹn → người viết khỏa thân chạy qua ranh giới trong một vòng lặp vô tận).
func TestCommitChapterLayeredAutoCompletesWhenDone(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Hai chương trong một tập và một arc, tất cả đều được mở ra (không có arc xương)
	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "Tập 1", "theme": "chủ đề",
			"arcs": []map[string]any{{
				"index": 1, "title": "cung một", "goal": "Mục tiêu",
				"chapters": []map[string]any{
					{"title": "Chương 1", "core_event": "tăng lên", "hook": "Tiếp tục"},
					{"title": "Chương tiếp theo", "core_event": "kế thừa", "hook": "kết thúc"},
				},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	// Dòng la bàn dài đã đóng lại (OpenThreads trống)
	if err := s.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "Nhân vật chính trở về nhà"}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	tool := NewCommitChapterTool(s)
	commit := func(ch int) map[string]any {
		if err := s.Drafts.SaveDraft(ch, fmt.Sprintf("Nội dung văn bản Chương %d, được sử dụng để kiểm tra mức độ hoàn thành xác định.", ch)); err != nil {
			t.Fatalf("SaveDraft %d: %v", ch, err)
		}
		args, _ := json.Marshal(map[string]any{
			"chapter": ch, "summary": "bản tóm tắt", "characters": []string{"nhân vật chính"}, "key_events": []string{"sự kiện"},
		})
		raw, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Execute ch%d: %v", ch, err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("Unmarshal ch%d: %v", ch, err)
		}
		return out
	}

	// Chương 1: Chưa xong, lẽ ra chưa xong
	if bc, _ := commit(1)["book_complete"].(bool); bc {
		t.Fatal("Kết thúc chương 1 không nên kích hoạt cái kết")
	}
	if p, _ := s.Progress.Load(); p.Phase == domain.PhaseComplete {
		t.Fatal("Sau khi viết Chapter 1 giai đoạn chưa xong")
	}

	// Chương 2 (chương cuối): sẽ tự động kết thúc
	if bc, _ := commit(2)["book_complete"].(bool); !bc {
		t.Fatal("Nó sẽ tự động kết thúc sau khi viết chương cuối cùng.")
	}
	if p, _ := s.Progress.Load(); p.Phase != domain.PhaseComplete {
		t.Fatalf("expected phase=complete, got %s", p.Phase)
	}
}

// TestCommitChapterLayeredNoAutoCompleteWithOpenThreads Xác minh tính bảo thủ: khi vẫn còn các chuỗi dài đang hoạt động
// Ngay cả khi chương này đã đầy, nó sẽ không tự động kết thúc, để lại quyền quyết định "có nên tiếp tục" cho kiến ​​​​trúc sư hay không.
func TestCommitChapterLayeredNoAutoCompleteWithOpenThreads(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "Tập 1", "theme": "chủ đề",
			"arcs": []map[string]any{{
				"index": 1, "title": "cung một", "goal": "Mục tiêu",
				"chapters": []map[string]any{{"title": "Chương 1", "core_event": "tăng lên", "hook": "Tiếp tục"}},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	// Vẫn còn những dòng dài hạn đang hoạt động chưa đóng
	if err := s.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "Nhân vật chính trở về nhà", OpenThreads: []string{"Kẻ thù cũ chưa bị tiêu diệt"}}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	if err := s.Drafts.SaveDraft(1, "Nội dung chính của chương duy nhất, nhưng dòng dài vẫn chưa kết thúc."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter": 1, "summary": "bản tóm tắt", "characters": []string{"nhân vật chính"}, "key_events": []string{"sự kiện"},
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if p, _ := s.Progress.Load(); p.Phase == domain.PhaseComplete {
		t.Fatal("Các dòng dài đang hoạt động không nên được tự động hoàn thành trước khi đóng.")
	}
}
