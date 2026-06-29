package diag

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// trọng điểm là một đoạn "văn bản mới" không bao giờ xuất hiện trong bản xuất.
const sentinel = "Vào một đêm tuyết rơi, nhân vật chính tiết lộ âm mưu chấn động của kẻ ác. Đây là văn bản mật."

// writeSession ghi một số thông báo vào thư mục đầu ra tạm thời ở định dạng session/*.jsonl.
func writeSession(t *testing.T, rel string, msgs []agentcore.Message) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta", "sessions", rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var b strings.Builder
	for _, m := range msgs {
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

func commitCall(chapterRaw string) agentcore.Message {
	args := json.RawMessage(`{"chapter":` + chapterRaw + `,"content":"` + sentinel + sentinel + `"}`)
	return agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.ToolCallBlock(agentcore.ToolCall{Name: "commit_chapter", Args: args})},
	}
}

func errResult(msg string) agentcore.Message {
	return agentcore.Message{
		Role:     agentcore.RoleTool,
		Content:  []agentcore.ContentBlock{agentcore.TextBlock(msg)},
		Metadata: map[string]any{"is_error": true},
	}
}

// TestExport_DeathLoopShape tái tạo end-to-end #34: Model chuyển đổi chương của commit_chapter
// Chuỗi hóa gây ra vòng lặp xác thực. Khẳng định rằng việc xuất có thể được định vị và văn bản mới không nằm ngoài gói.
func TestExport_DeathLoopShape(t *testing.T) {
	var msgs []agentcore.Message
	// Văn bản kế hoạch điều phối trần (<4KB, bỏ qua session_compact) phải được mã hóa.
	msgs = append(msgs, agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.TextBlock(sentinel)},
	})
	// Vòng 14 commit_chapter(chapter:"7") + inputValidationError.
	for range 14 {
		msgs = append(msgs, commitCall(`"7"`))
		msgs = append(msgs, errResult("InputValidationError: chapter must be int"))
	}

	dir := writeSession(t, "coordinator.jsonl", msgs)
	s := store.NewStore(dir)
	rep, rc := Diagnose(s)
	out := string(RenderExport(rep, rc))

	if strings.Contains(out, sentinel) {
		t.Fatalf("Văn bản của cuốn tiểu thuyết đã ra mắt! Bản xuất có chứa trọng điểm:\n%s", out)
	}
	if !strings.Contains(out, `chapter: "7"`) {
		t.Errorf("Chương tín hiệu ngoại lệ loại bị thiếu: \"7\" (nguyên nhân gốc #34) \n%s", out)
	}
	if !strings.Contains(out, "InputValidationError") {
		t.Errorf("Chuỗi lỗi \n%s không được giữ lại", out)
	}
	if !strings.Contains(out, "×14") {
		t.Errorf("Tập hợp lặp lại không được liệt kê ×14\n%s", out)
	}
	// Giai đoạn 2: Việc phát hiện thời gian chạy sẽ xác định vòng lặp này là RepeatedToolError nghiêm trọng.
	if !strings.Contains(out, "Công cụ báo lỗi tương tự nhiều lần") {
		t.Errorf("Phát hiện thời gian chạy không tạo ra RepeatedToolError\n%s", out)
	}
	if !strings.Contains(out, "[critical]") {
		t.Errorf("14 lần lặp lại nên được nâng lên tới mức quan trọng\n%s", out)
	}
}

// TestExport_NumberVsStringArg chứng minh rằng phép chiếu vô hướng và chuỗi có thể phân biệt các loại:
// chương:7 (số) được giữ lại là 7, chương:"7" (chuỗi) được giữ lại là "7".
func TestExport_NumberVsStringArg(t *testing.T) {
	intDir := writeSession(t, "coordinator.jsonl", []agentcore.Message{commitCall(`7`)})
	si := store.NewStore(intDir)
	repInt, rcInt := Diagnose(si)
	outInt := string(RenderExport(repInt, rcInt))
	if !strings.Contains(outInt, "chapter: 7") || strings.Contains(outInt, `chapter: "7"`) {
		t.Errorf("Đối số dạng số phải được hiển thị dưới dạng chương: 7 (không có dấu ngoặc kép) \n%s", outInt)
	}
}

// TestProjectValue_ProseArgRedacted bảo vệ ranh giới giải mẫn cảm: lưu giữ giá trị ngắn loại mã định danh,
// Các giá trị tiếng Trung/ngắn có dấu cách (chẳng hạn như nhiệm vụ điều phối, tiêu đề chương) luôn được mã hóa.
func TestProjectValue_ProseArgRedacted(t *testing.T) {
	keep := map[string]string{
		`"7"`:       `"7"`,       // Xâu chuỗi số (tín hiệu #34)
		`"premise"`: `"premise"`, // liệt kê
		`"writer"`:  `"writer"`,  // Tên nhân vật
		`7`:         `7`,         // vô hướng số
		`true`:      `true`,      // vô hướng bool
	}
	for in, want := range keep {
		if got := projectValue([]byte(in)); got != want {
			t.Errorf("Nên giữ lại %s: đã có %q muốn %q", in, got, want)
		}
	}
	// Chứa tiếng Trung/dấu cách → phải được mã hóa và không chứa văn bản gốc.
	prose := []string{`"Chương 7 Sự thật về đêm tuyết"`, `"Sát nhân đêm tuyết"`, `"Nhân vật chính vạch trần âm mưu"`}
	for _, in := range prose {
		got := projectValue([]byte(in))
		if !strings.HasPrefix(got, "<redacted") {
			t.Errorf("Các giá trị tiếng Trung/ngắn có dấu cách phải được mã hóa: %s → %q", in, got)
		}
		if strings.Contains(got, "đêm tuyết rơi") || strings.Contains(got, "nhân vật chính") {
			t.Errorf("Văn bản chính vẫn được đưa vào sau khi mã hóa: %s → %q", in, got)
		}
	}
}

// TestWriteExport_WritesFile chứng minh các đường dẫn hàm thuần túy: không dựa vào TUI và ghi các đường dẫn tương đối cố định.
func TestWriteExport_WritesFile(t *testing.T) {
	dir := writeSession(t, "coordinator.jsonl", []agentcore.Message{commitCall(`"7"`), errResult("boom")})
	s := store.NewStore(dir)

	rep, rc := Diagnose(s)
	path, err := WriteExport(s, rep, rc)
	if err != nil {
		t.Fatalf("WriteExport: %v", err)
	}
	if want := filepath.Join(dir, filepath.FromSlash(ExportRelPath)); path != want {
		t.Errorf("Đường dẫn sai: có %s muốn %s", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(data), "diag-export") {
		t.Errorf("Nội dung file bất thường \n%s", data)
	}
	if strings.Contains(string(data), sentinel) {
		t.Errorf("Thư mục viết có văn bản")
	}
}

// TestRedactMessage_DupSha chứng minh rằng các lần xuất hiện lặp đi lặp lại của cùng một văn bản sẽ tạo ra cùng một sha (tín hiệu tuần hoàn).
func TestRedactMessage_DupSha(t *testing.T) {
	a := redactMessage("coordinator", agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.TextBlock(sentinel)},
	})
	b := redactMessage("coordinator", agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.TextBlock(sentinel)},
	})
	if a.TextSha == "" || a.TextSha != b.TextSha {
		t.Errorf("Cùng một cơ thể xứng đáng sha: %q vs %q", a.TextSha, b.TextSha)
	}
	if a.Redacted != 1 {
		t.Errorf("1 khối văn bản phải được mã hóa, có %d", a.Redacted)
	}
}
