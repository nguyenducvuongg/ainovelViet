package host

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/voocel/agentcore"
)

// sessionRecord là một dạng phân tích cú pháp nhẹ của một bản ghi trong meta/sessions/*.jsonl - chỉ tìm nạp
// Trường bắt buộc để sử dụng tích lũy. Các trường lớn như Nội dung bỏ qua phân tích cú pháp, lưu IO trong khi khởi động.
//
// Mô hình được hạ cấp xuống ba cấp độ:
//  1. Cách sử dụng.Provider/Model - mô hình phản hồi thực truyền tải trong suốt của Agentcore/litellm (ưu tiên)
//  2. Meta(_meta) — Khi luồng ngược dòng không được truyền đi một cách minh bạch, mô hình "hợp lệ tại thời điểm đó" được ModelLookup thêm vào ở phía ghi
//  3. Không - phát lại trả về mô hình hiệu quả và sử dụng ModelSet hiện tại để suy luận (độ chính xác bị suy giảm)
type sessionRecord struct {
	Role  agentcore.Role     `json:"role"`
	Usage *agentcore.Usage   `json:"usage,omitempty"`
	Meta  *sessionRecordMeta `json:"_meta,omitempty"`
}

type sessionRecordMeta struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// ReplaySessions quét meta/sessions/điều phối viên.jsonl và meta/sessions/agents/*.jsonl,
// Tích lũy việc sử dụng từng tin nhắn trợ lý trở lại trình theo dõi. Trả về số lượng mục chèn lấp.
//
// Ràng buộc cuộc gọi: chỉ gọi một lần khi thiếu meta/usage.json (nâng cấp lần đầu hoặc thay đổi lược đồ), hãy thực hiện
// Chèn lấp dữ liệu lịch sử. Sự kiên trì hàng ngày sẽ chuyển đến SaveNow/autoSaveLoop.
//
// Để biết sự phụ thuộc chính xác, hãy xem phần hạ cấp ba cấp của chú thích sessionRecord - Cấp 3 (thiếu cả Cách sử dụng và _meta)
// Nó sẽ chỉ được kích hoạt khi xảy ra các nhật ký cũ hơn hoặc ngoại lệ ngược dòng.
func (t *UsageTracker) ReplaySessions(rootDir string) (int, error) {
	if t == nil {
		return 0, nil
	}
	sessionsDir := filepath.Join(rootDir, "meta", "sessions")
	info, err := os.Stat(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, nil
	}

	total := 0
	if n, err := t.replayFile(filepath.Join(sessionsDir, "coordinator.jsonl"), "coordinator"); err != nil {
		slog.Warn("replay coordinator session failed", "module", "usage", "err", err)
	} else {
		total += n
	}

	agentsDir := filepath.Join(sessionsDir, "agents")
	walkErr := filepath.WalkDir(agentsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		agentName := parseAgentNameFromFile(name)
		if agentName == "" {
			return nil
		}
		n, fileErr := t.replayFile(path, agentName)
		if fileErr != nil {
			slog.Warn("replay agent session failed", "module", "usage", "file", name, "err", fileErr)
			return nil
		}
		total += n
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return total, walkErr
	}
	return total, nil
}

// replayFile quét một tệp jsonl duy nhất và cung cấp tất cả các tin nhắn trợ lý có Mức sử dụng để tích lũy.
// AgentName được người gọi chuyển vào (tên điều phối viên hoặc tên tác nhân phụ để phân giải tên tệp).
func (t *UsageTracker) replayFile(path, agentName string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	role := agentRoleName(agentName)
	count := 0
	scanner := bufio.NewScanner(f)
	// Một dòng có thể rất dài (tin nhắn hỗ trợ + đối số công cụ, v.v. đều bằng phẳng), hãy giảm xuống còn 4MB.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec sessionRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Role != agentcore.RoleAssistant || rec.Usage == nil {
			continue
		}
		provider, modelName := usageActualModel(rec.Usage)
		if rec.Meta != nil {
			if provider == "" {
				provider = rec.Meta.Provider
			}
			if modelName == "" {
				modelName = rec.Meta.Model
			}
		}
		t.accumulate(role, provider, modelName, *rec.Usage)
		count++
	}
	if err := scanner.Err(); err != nil {
		return count, fmt.Errorf("scan %s: %w", path, err)
	}
	return count, nil
}

// ParseAgentNameFromFile được trích xuất từ ​​"writer-ch01.jsonl" / "architect_short-001.jsonl"
// tên đại lý (phần trước "-"). Xem store/session.go::subAgentPath để biết quy ước đặt tên:
// AgentName không chứa dấu gạch ngang, hậu tố là ch<n> hoặc số thứ tự tăng dần.
func parseAgentNameFromFile(name string) string {
	base := strings.TrimSuffix(name, ".jsonl")
	if i := strings.Index(base, "-"); i > 0 {
		return base[:i]
	}
	return ""
}
