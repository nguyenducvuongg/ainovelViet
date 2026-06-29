package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/voocel/agentcore"
)

// SessionStore nối thêm lịch sử hội thoại LLM vào tệp JSONL.
// Nội dung lớn hơn (văn bản mới, ngữ cảnh đầy đủ) được thay thế bằng thẻ giữ chỗ [session_compact: ...].
type SessionStore struct {
	io      *IO
	mu      sync.Mutex
	seq     map[string]int    // Tác nhân đang chạy số sê-ri (được sử dụng khi không thể trích xuất số chương)
	taskKey map[string]string // "agentName|task" → hậu tố, cùng một lần chạy sẽ sử dụng lại cùng một tệp
}

func NewSessionStore(io *IO) *SessionStore {
	return &SessionStore{io: io, seq: make(map[string]int), taskKey: make(map[string]string)}
}

// ModelLookup kiểm tra nhà cung cấp/mô hình "hiện có hiệu lực" theo tên tác nhân khi ghi nhật ký.
// Sử dụng loại func thay vì giao diện để tạo điều kiện cho người gọi sử dụng các bao đóng để đưa vào các quy tắc chuẩn hóa (chẳng hạn như Architect_short → Architect).
// Trả về một chuỗi trống có nghĩa là không xác định, người gọi vẫn viết như bình thường nhưng không có _meta và quay lại dự phòng ModelSet khi phát lại.
type ModelLookup func(agentName string) (provider, model string)

// Điều phối viênLogger trả về lệnh gọi lại OnMessage của điều phối viên.
// tra cứu có thể bằng 0, trong trường hợp đó nó được viết mà không có _meta (tương thích với các cảnh không có vai trò như cocreate).
func (s *SessionStore) CoordinatorLogger(lookup ModelLookup) func(agentcore.AgentMessage) {
	return func(msg agentcore.AgentMessage) {
		var meta *sessionLogMeta
		if lookup != nil {
			meta = lookupMeta(lookup, "coordinator")
		}
		if err := s.logEntry("meta/sessions/coordinator.jsonl", msg, meta); err != nil {
			slog.Warn("session log failed", "agent", "coordinator", "err", err)
		}
	}
}

// SubAgentLogger Trả về lệnh gọi lại OnMessage của tác nhân phụ.
func (s *SessionStore) SubAgentLogger(lookup ModelLookup) func(agentName, task string, msg agentcore.AgentMessage) {
	return func(agentName, task string, msg agentcore.AgentMessage) {
		rel := s.subAgentPath(agentName, task)
		var meta *sessionLogMeta
		if lookup != nil {
			meta = lookupMeta(lookup, agentName)
		}
		if err := s.logEntry(rel, msg, meta); err != nil {
			slog.Warn("session log failed", "agent", agentName, "err", err)
		}
	}
}

func lookupMeta(lookup ModelLookup, agentName string) *sessionLogMeta {
	provider, model := lookup(agentName)
	if provider == "" && model == "" {
		return nil
	}
	return &sessionLogMeta{Provider: provider, Model: model}
}

// LogCoCreate thêm nhật ký cuộc trò chuyện đồng sáng tạo vào meta/sessions/cocreate.jsonl.
// Trong giai đoạn đồng sáng tạo, các tiểu thuyết cụ thể vẫn chưa bị ràng buộc và tất cả chúng đều được đặt dưới gốc mặc định là OutputDir (đầu ra/tiểu thuyết).
// Vị trí giống như địa chỉ điều phối viên.jsonl/agents/* được tạo chính thức để dễ dàng khắc phục sự cố.
func (s *SessionStore) LogCoCreate(entry any) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cocreate session: %w", err)
	}
	data = append(data, '\n')
	return s.io.AppendLine("meta/sessions/cocreate.jsonl", data)
}

// Nhật ký sẽ thêm thông báo vào đường dẫn đã chỉ định và tự động nén nội dung lớn.
// Không mang _meta (mục tương thích ngược; chỉ được sử dụng cho các đường dẫn không có vai trò như cocreate).
func (s *SessionStore) Log(rel string, msg agentcore.AgentMessage) error {
	return s.logEntry(rel, msg, nil)
}

// sessionLogEntry nhúng Agentcore.Message + _meta tùy chọn.
// Agentcore.Message là cấu trúc đơn giản (không có MarshalJSON), được nhúng trong json marshal
// Tự động mở rộng lên cấp cao nhất; _meta được điều khiển thông qua omitempty - chỉ trợ lý + Cách sử dụng != nil
// Nó chỉ được tiêm khi nó được sử dụng. Thông báo người dùng/công cụ không mang _meta. Khi phân tích cú pháp jsonl cũ, _meta=nil là một lỗi.
type sessionLogEntry struct {
	agentcore.Message
	Meta *sessionLogMeta `json:"_meta,omitempty"`
}

type sessionLogMeta struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// logEntry tuần tự hóa tin nhắn và thêm _meta nếu cần. lookupMeta Meta được tính toán được chuyển vào;
// Đánh giá nội bộ của hàm chỉ ghi meta cho thông báo "cách sử dụng LLM đã tạo" (trợ lý + Cách sử dụng != nil).
// Các tin nhắn khác vẫn ở dạng tuần tự hóa Agentcore.Message.
func (s *SessionStore) logEntry(rel string, msg agentcore.AgentMessage, meta *sessionLogMeta) error {
	m, ok := msg.(agentcore.Message)
	if !ok {
		return nil // Các tin nhắn không phải LLM (chẳng hạn như các loại tùy chỉnh) bị bỏ qua
	}
	compacted := compactMessage(m)
	entry := sessionLogEntry{Message: compacted}
	if compacted.Role == agentcore.RoleAssistant && compacted.Usage != nil {
		entry.Meta = usageMeta(compacted.Usage)
		if entry.Meta == nil {
			entry.Meta = meta
		}
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal session message: %w", err)
	}
	data = append(data, '\n')
	return s.io.AppendLine(rel, data)
}

func usageMeta(usage *agentcore.Usage) *sessionLogMeta {
	if usage == nil || (usage.Provider == "" && usage.Model == "") {
		return nil
	}
	return &sessionLogMeta{
		Provider: usage.Provider,
		Model:    usage.Model,
	}
}

// subAgentPath tạo đường dẫn tệp dựa trên tác vụ AgentName+.
func (s *SessionStore) subAgentPath(agentName, task string) string {
	suffix := extractChapter(task)
	if suffix != "" {
		return fmt.Sprintf("meta/sessions/agents/%s-%s.jsonl", agentName, suffix)
	}
	key := agentName + "|" + task
	s.mu.Lock()
	if cached, ok := s.taskKey[key]; ok {
		s.mu.Unlock()
		return fmt.Sprintf("meta/sessions/agents/%s-%s.jsonl", agentName, cached)
	}
	s.seq[agentName]++
	suffix = fmt.Sprintf("%03d", s.seq[agentName])
	s.taskKey[key] = suffix
	s.mu.Unlock()
	return fmt.Sprintf("meta/sessions/agents/%s-%s.jsonl", agentName, suffix)
}

var chapterRe = regexp.MustCompile(`(?i)chương\s*(\d+)`)

func extractChapter(task string) string {
	m := chapterRe.FindStringSubmatch(task)
	if len(m) < 2 {
		return ""
	}
	n, _ := strconv.Atoi(m[1])
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("ch%02d", n)
}

// compactMessage sao chép tin nhắn và thay thế nội dung lớn.
func compactMessage(m agentcore.Message) agentcore.Message {
	if len(m.Content) == 0 {
		return m
	}
	blocks := make([]agentcore.ContentBlock, len(m.Content))
	copy(blocks, m.Content)

	toolName := toolNameFromMeta(m.Metadata)

	for i := range blocks {
		switch blocks[i].Type {
		case agentcore.ContentText:
			blocks[i].Text = compactText(m.Role, toolName, blocks[i].Text)
		case agentcore.ContentToolCall:
			if blocks[i].ToolCall != nil {
				blocks[i].ToolCall = compactToolCall(blocks[i].ToolCall)
			}
		}
	}
	m.Content = blocks
	return m
}

func toolNameFromMeta(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta["tool_name"].(string); ok {
		return v
	}
	return ""
}

// compactText nén nội dung văn bản của kết quả công cụ.
func compactText(role agentcore.Role, toolName, text string) string {
	if role != agentcore.RoleTool || len(text) < 4096 {
		return text
	}
	switch toolName {
	case "novel_context":
		summary := extractJSONField(text, "_loading_summary")
		return fmt.Sprintf("[session_compact: novel_context %dB | %s]", len(text), summary)
	case "read_chapter":
		chars := utf8.RuneCountInString(text)
		return fmt.Sprintf("[session_compact: read_chapter %d từ | xem chương/]", chars)
	default:
		if len(text) > 8192 {
			chars := utf8.RuneCountInString(text)
			return fmt.Sprintf("[session_compact: %s %d từ]", toolName, chars)
		}
		return text
	}
}

// compactToolCall nén các trường nội dung lớn trong đối số của lệnh gọi công cụ.
func compactToolCall(tc *agentcore.ToolCall) *agentcore.ToolCall {
	switch tc.Name {
	case "draft_chapter":
		return compactArgsContent(tc, "Chương N văn bản", "drafts/")
	case "save_foundation":
		return compactFoundationArgs(tc)
	default:
		return tc
	}
}

func compactArgsContent(tc *agentcore.ToolCall, label, ref string) *agentcore.ToolCall {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return tc
	}
	contentRaw, ok := args["content"]
	if !ok || len(contentRaw) < 4096 {
		return tc
	}
	var content string
	if err := json.Unmarshal(contentRaw, &content); err != nil {
		// nội dung không phải là một chuỗi (có thể là đối tượng JSON), tính bằng byte
		placeholder := fmt.Sprintf("[session_compact: %s %dB | xem %s]", label, len(contentRaw), ref)
		args["content"], _ = json.Marshal(placeholder)
	} else {
		chars := utf8.RuneCountInString(content)
		ch := extractJSONFieldInt(tc.Args, "chapter")
		if ch > 0 {
			label = fmt.Sprintf("Chương %d Văn bản", ch)
			ref = fmt.Sprintf("drafts/%02d.draft.md", ch)
		}
		placeholder := fmt.Sprintf("[session_compact: %s %d từ | xem %s]", label, chars, ref)
		args["content"], _ = json.Marshal(placeholder)
	}
	clone := *tc
	clone.Args, _ = json.Marshal(args)
	return &clone
}

func compactFoundationArgs(tc *agentcore.ToolCall) *agentcore.ToolCall {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return tc
	}
	contentRaw, ok := args["content"]
	if !ok || len(contentRaw) < 4096 {
		return tc
	}
	typeName := "foundation"
	var t string
	if json.Unmarshal(args["type"], &t) == nil && t != "" {
		typeName = t
	}
	placeholder := fmt.Sprintf("[session_compact: %s %dB | xem cửa hàng]", typeName, len(contentRaw))
	args["content"], _ = json.Marshal(placeholder)
	clone := *tc
	clone.Args, _ = json.Marshal(args)
	return &clone
}

// extractJSONField trích xuất giá trị chuỗi của trường được chỉ định từ chuỗi JSON.
func extractJSONField(jsonStr, field string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return ""
	}
	raw, ok := m[field]
	if !ok {
		return ""
	}
	var val string
	if err := json.Unmarshal(raw, &val); err != nil {
		return string(raw)
	}
	return val
}

func extractJSONFieldInt(data json.RawMessage, field string) int {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return 0
	}
	raw, ok := m[field]
	if !ok {
		return 0
	}
	var val int
	if err := json.Unmarshal(raw, &val); err != nil {
		return 0
	}
	return val
}

// CompactTag là tiền tố thẻ giữ chỗ để hỗ trợ tìm kiếm và khôi phục.
const CompactTag = "[session_compact:"

// IsCompacted Kiểm tra xem văn bản đã được nén hay chưa.
func IsCompacted(text string) bool {
	return strings.HasPrefix(text, CompactTag)
}
