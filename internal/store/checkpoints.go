package store

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
)

const checkpointsFile = "meta/checkpoints.jsonl"

// CheckpointStore quản lý việc bổ sung và truy vấn các điểm kiểm tra cấp độ.
// Định dạng đĩa: meta/checkpoints.jsonl, chỉ nối thêm; truy vấn hình ảnh bộ nhớ.
// Bất biến: bộ đệm là bản sao của checkpoints.jsonl và được Append/Reset duy trì tại một điểm duy nhất.
// Đồng thời: Bộ đệm được bảo vệ bởi io.mu và Khóa được sử dụng để ghi và RLock được sử dụng để đọc.
type CheckpointStore struct {
	io     *IO
	seqGen atomic.Int64
	cache  []domain.Checkpoint
}

// NewCheckpointStore tạo bộ lưu trữ điểm kiểm tra và tải các điểm kiểm tra hiện có từ đĩa vào bộ đệm cùng một lúc.
func NewCheckpointStore(io *IO) *CheckpointStore {
	cs := &CheckpointStore{io: io}
	cs.loadFromDisk()
	return cs
}

// LoadFromDisk đọc đĩa jsonl vào bộ đệm và khôi phục seqGen ngay lập tức.
func (cs *CheckpointStore) loadFromDisk() {
	cs.io.mu.Lock()
	defer cs.io.mu.Unlock()

	cs.cache = readCheckpointsFile(cs.io.path(checkpointsFile))
	var maxSeq int64
	for _, cp := range cs.cache {
		if cp.Seq > maxSeq {
			maxSeq = cp.Seq
		}
	}
	cs.seqGen.Store(maxSeq)
}

// Nối thêm một điểm kiểm tra.
// Idempotent: Tương tự như Phạm vi + Bước + Thông báo. Nếu bản ghi đã tồn tại, bỏ qua việc ghi và trực tiếp trả lại bản ghi hiện có.
func (cs *CheckpointStore) Append(scope domain.Scope, step, artifact, digest string) (*domain.Checkpoint, error) {
	cs.io.mu.Lock()
	defer cs.io.mu.Unlock()

	if digest != "" {
		for i := len(cs.cache) - 1; i >= 0; i-- {
			cp := cs.cache[i]
			if cp.Scope.Matches(scope) && cp.Step == step && cp.Digest == digest {
				return &cp, nil
			}
		}
	}

	// Seq chỉ được nâng cao sau khi nó được viết thành công để tránh các bước nhảy vĩnh viễn do ghi không thành công.
	// Khóa ghi io.mu đã được giữ và Load+Store sẽ không được ưu tiên đồng thời.
	seq := cs.seqGen.Load() + 1
	cp := domain.Checkpoint{
		Seq:        seq,
		Scope:      scope,
		Step:       step,
		Artifact:   artifact,
		Digest:     digest,
		OccurredAt: time.Now(),
	}

	data, err := json.Marshal(cp)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := cs.io.AppendLineUnlocked(checkpointsFile, data); err != nil {
		return nil, err
	}
	cs.seqGen.Store(seq)
	cs.cache = append(cs.cache, cp)
	return &cp, nil
}

// AppendArtifact tính toán dấu vân tay nội dung giả tạo và thêm điểm kiểm tra.
func (cs *CheckpointStore) AppendArtifact(scope domain.Scope, step, artifact string) (*domain.Checkpoint, error) {
	if artifact == "" {
		return cs.Append(scope, step, "", "")
	}
	data, err := cs.io.ReadFile(artifact)
	if err != nil {
		return nil, fmt.Errorf("digest artifact %s: %w", artifact, err)
	}
	sum := sha256.Sum256(data)
	return cs.Append(scope, step, artifact, "sha256:"+hex.EncodeToString(sum[:]))
}

// Mới nhất trả về điểm kiểm tra mới nhất cho phạm vi được chỉ định.
func (cs *CheckpointStore) Latest(scope domain.Scope) *domain.Checkpoint {
	cs.io.mu.RLock()
	defer cs.io.mu.RUnlock()
	for i := len(cs.cache) - 1; i >= 0; i-- {
		if cs.cache[i].Scope.Matches(scope) {
			cp := cs.cache[i]
			return &cp
		}
	}
	return nil
}

// LastByStep trả về điểm kiểm tra mới nhất của phạm vi + bước được chỉ định.
func (cs *CheckpointStore) LatestByStep(scope domain.Scope, step string) *domain.Checkpoint {
	cs.io.mu.RLock()
	defer cs.io.mu.RUnlock()
	for i := len(cs.cache) - 1; i >= 0; i-- {
		cp := cs.cache[i]
		if cp.Scope.Matches(scope) && cp.Step == step {
			return &cp
		}
	}
	return nil
}

// LastGlobal trả về điểm kiểm tra toàn cầu mới nhất (không phân biệt giữa các phạm vi).
func (cs *CheckpointStore) LatestGlobal() *domain.Checkpoint {
	cs.io.mu.RLock()
	defer cs.io.mu.RUnlock()
	if len(cs.cache) == 0 {
		return nil
	}
	cp := cs.cache[len(cs.cache)-1]
	return &cp
}

// Tất cả trả về một bản sao của toàn bộ danh sách điểm kiểm tra (tăng theo seq).
func (cs *CheckpointStore) All() []domain.Checkpoint {
	cs.io.mu.RLock()
	defer cs.io.mu.RUnlock()
	if len(cs.cache) == 0 {
		return nil
	}
	out := make([]domain.Checkpoint, len(cs.cache))
	copy(out, cs.cache)
	return out
}

// Đặt lại sẽ xóa tệp điểm kiểm tra và bộ đệm. Chỉ được sử dụng khi tạo một cuốn tiểu thuyết mới.
// Xóa tệp trước rồi xóa bộ nhớ: giữ lại bộ đệm và seqGen khi xóa không thành công để tránh tình trạng bộ nhớ và ổ đĩa bị sai lệch.
func (cs *CheckpointStore) Reset() error {
	cs.io.mu.Lock()
	defer cs.io.mu.Unlock()
	if err := cs.io.RemoveFileUnlocked(checkpointsFile); err != nil {
		return err
	}
	cs.seqGen.Store(0)
	cs.cache = nil
	return nil
}

// readCheckpointsFile phân tích cú pháp jsonl; bỏ qua các dòng không đúng định dạng để chấp nhận việc cắt đuôi.
func readCheckpointsFile(path string) []domain.Checkpoint {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var result []domain.Checkpoint
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var cp domain.Checkpoint
		if json.Unmarshal(line, &cp) == nil {
			result = append(result, cp)
		}
	}
	return result
}
