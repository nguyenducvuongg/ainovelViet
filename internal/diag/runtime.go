package diag

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

const (
	logTailCap   = 200 << 10 // Chỉ lấy 200KB cuối cùng của nhật ký (vòng lặp là hiện tượng gần kết thúc)
	sessionTail  = 80        // Số lượng đuôi xương (tùy theo thứ tự phân bố)
	repeatWindow = 150       // Việc tổng hợp lặp đi lặp lại chỉ xem xét rất nhiều sự kiện sắp kết thúc - các công cụ thông thường tích lũy hàng trăm lần khi chạy đường dài.
	// Sự tuần hoàn thực sự tập trung cao độ ở đầu gần; cửa sổ được sử dụng thay vì tích lũy để tránh đánh giá sai "tiến bộ bình thường" là "vòng lặp vô hạn".
	recentAgents = 2  // Quét bổ sung để biết số phiên tác nhân phụ đang hoạt động gần đây
	repeatMin    = 3  // Số lần lặp lại được coi là "tín hiệu tần số cao"
	repeatTopN   = 12 // Số lượng chữ ký trùng lặp tối đa
)

// RuntimeCapture là kết quả giải mẫn cảm của quá trình chụp trong thời gian chạy. Chỉ mang tín hiệu thời gian chạy;
// Các trạng thái tạo như giai đoạn/luồng/chương được Report.Stats thực hiện và sẽ không được lặp lại ở đây.
type RuntimeCapture struct {
	GoOS, GoArch  string
	Models        []RoleModel  // Nhà cung cấp/mô hình thực tế có hiệu lực cho mỗi phiên (được thu thập từ _meta)
	CurrentStep   string       // Điểm kiểm tra mới nhất: phạm vi.step
	StuckStep     string       // Đuôi liên tục với cùng một bước; "" = không bị kẹt
	StuckCount    int          // lần liên tiếp
	Repeats       []RepeatStat // Lặp lại chữ ký top-N (tín hiệu tuần hoàn)
	DupContent    []DupStat    // Văn bản sha giống nhau xuất hiện lặp đi lặp lại (cùng một đoạn được tạo lặp đi lặp lại)
	LogKinds      map[string]int
	LogErrors     int
	LogWarns      int
	StopGuard     int
	Tail          []SkelEvent // N bộ xương cuối cùng (tuỳ theo thứ tự)
	RedactedTexts int         // Tổng số khối văn bản được mã hóa (tự kiểm tra giải mẫn cảm)
	Sources       []string    // Nguồn thực sự đọc (tự kiểm tra)
}

// RoleModel ghi lại nhà cung cấp/mô hình thực tế được phiên sử dụng.
type RoleModel struct {
	Agent, Provider, Model string
}

// RepeatStat là chữ ký lặp lại và số lần của nó.
type RepeatStat struct {
	Sig   string
	Count int
}

// DupStat là số lần cùng một văn bản được giải mẫn cảm xuất hiện lặp đi lặp lại.
type DupStat struct {
	Sha   string
	Count int
}

// sessionLine phân tích một dòng trong phiên/*.jsonl: nhúng Agentcore.Message + _meta tùy chọn.
type sessionLine struct {
	agentcore.Message
	Meta *struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	} `json:"_meta"`
}

var kindRe = regexp.MustCompile(`kind=(\S+)`)

// CaptureRuntime read-only ghi lại các tín hiệu thời gian chạy từ thư mục đầu ra và làm giảm độ nhạy của tập hợp.
// Bất kỳ nguồn nào bị thiếu sẽ được hạ cấp một cách an toàn (không có lỗi được báo cáo), nỗ lực hết mình.
func CaptureRuntime(s *store.Store) RuntimeCapture {
	rc := RuntimeCapture{GoOS: runtime.GOOS, GoArch: runtime.GOARCH, LogKinds: map[string]int{}}

	rc.CurrentStep, rc.StuckStep, rc.StuckCount = analyzeCheckpoints(s.Checkpoints.All())
	captureSessions(s.Dir(), &rc)
	captureLog(s.Dir(), &rc)
	return rc
}

// analyzeCheckpoints thực hiện bước mới nhất và tính toán bước liên tiếp cuối cùng của cùng một bước (tín hiệu bị kẹt).
func analyzeCheckpoints(cps []domain.Checkpoint) (current, stuck string, count int) {
	if len(cps) == 0 {
		return "", "", 0
	}
	key := func(c domain.Checkpoint) string { return fmt.Sprintf("%s.%s", c.Scope, c.Step) }
	current = key(cps[len(cps)-1])
	n := 1
	for i := len(cps) - 2; i >= 0; i-- {
		if key(cps[i]) == current {
			n++
		} else {
			break
		}
	}
	if n >= repeatMin {
		stuck, count = current, n
	}
	return current, stuck, count
}

// Điều phối viên quét captureSessions + các phiên tác nhân phụ gần đây, tập hợp mặt nạ.
func captureSessions(dir string, rc *RuntimeCapture) {
	sessDir := filepath.Join(dir, "meta", "sessions")
	files := sessionFiles(sessDir)

	repeats := map[string]int{}
	dups := map[string]int{}
	models := map[string]RoleModel{}

	for _, f := range files {
		evs := scanSession(filepath.Join(sessDir, f.path), f.agent, rc, models)
		// Việc tổng hợp chỉ nhìn vào cửa sổ gần kết thúc: khi chạy đường dài, subagent/novel_context tích lũy hàng trăm lần và được quảng bá bình thường.
		// Đó không phải là một chu kỳ; một chu kỳ vô tận thực sự tập trung cao độ ở đầu gần.
		aggregateRepeats(f.agent, tailEvents(evs, repeatWindow), repeats, dups)
		// Đuôi bộ xương được ưu tiên hơn bộ điều phối - vòng lặp điều phối có thể được nhìn thấy rõ ràng nhất ở đây.
		if f.agent == "coordinator" && len(evs) > 0 {
			rc.Tail = tailEvents(evs, sessionTail)
		}
		rc.Sources = append(rc.Sources, "sessions/"+f.path)
	}
	if len(rc.Tail) == 0 {
		// Quay trở lại tác nhân con gần đây nhất khi không có phiên điều phối viên.
		for _, f := range files {
			if evs := scanSessionTailOnly(filepath.Join(sessDir, f.path), f.agent); len(evs) > 0 {
				rc.Tail = tailEvents(evs, sessionTail)
				break
			}
		}
	}

	rc.Repeats = topRepeats(repeats)
	rc.DupContent = topDups(dups)
	rc.Models = sortedModels(models)
}

type sessionFile struct {
	path  string // Liên quan đến sessDir
	agent string
}

// sessionFiles trả về coctor.jsonl + các phiên tác nhân phụ hoạt động gần đây nhất.
func sessionFiles(sessDir string) []sessionFile {
	var out []sessionFile
	if _, err := os.Stat(filepath.Join(sessDir, "coordinator.jsonl")); err == nil {
		out = append(out, sessionFile{path: "coordinator.jsonl", agent: "coordinator"})
	}

	agentsDir := filepath.Join(sessDir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return out
	}
	type withTime struct {
		name string
		mod  int64
	}
	var agents []withTime
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if info, err := e.Info(); err == nil {
			agents = append(agents, withTime{e.Name(), info.ModTime().UnixNano()})
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].mod > agents[j].mod })
	for i, a := range agents {
		if i >= recentAgents {
			break
		}
		stem := strings.TrimSuffix(a.name, ".jsonl")
		out = append(out, sessionFile{path: filepath.Join("agents", a.name), agent: stem})
	}
	return out
}

// scanSession đọc tệp phiên, giải mẫn cảm từng dòng một và thu thập các chuỗi sự kiện và mô hình trên mỗi tác nhân.
// Việc tổng hợp lặp lại/cùng phân đoạn không được thực hiện ở đây - hãy để nó ở dạng tổng hợpLặp lại để được tính toán trên cửa sổ gần cuối.
func scanSession(path, agent string, rc *RuntimeCapture, models map[string]RoleModel) []SkelEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var evs []SkelEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		var sl sessionLine
		if json.Unmarshal(sc.Bytes(), &sl) != nil {
			continue
		}
		ev := redactMessage(agent, sl.Message)
		evs = append(evs, ev)
		rc.RedactedTexts += ev.Redacted
		if sl.Meta != nil && (sl.Meta.Provider != "" || sl.Meta.Model != "") {
			models[agent] = RoleModel{Agent: agent, Provider: sl.Meta.Provider, Model: sl.Meta.Model}
		}
	}
	return evs
}

// tổng hợpLặp lại tích lũy các chữ ký trùng lặp và cùng một văn bản trên một cửa sổ sự kiện nhất định.
func aggregateRepeats(agent string, evs []SkelEvent, repeats, dups map[string]int) {
	for _, ev := range evs {
		for _, t := range ev.Tools {
			sig := agent + " · " + t.Name
			if t.Invalid {
				sig += " (args invalid)"
			}
			repeats[sig]++
		}
		if ev.ErrClass != "" {
			repeats[agent+" · err: "+ev.ErrClass]++
		}
		if ev.TextSha != "" {
			dups[ev.TextSha]++
		}
	}
}

// scanSessionTailOnly chỉ lấy bộ xương (không tính tổng hợp), được sử dụng để cung cấp phần đuôi khi thiếu điều phối viên.
func scanSessionTailOnly(path, agent string) []SkelEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var evs []SkelEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		var sl sessionLine
		if json.Unmarshal(sc.Bytes(), &sl) != nil {
			continue
		}
		evs = append(evs, redactMessage(agent, sl.Message))
	}
	return evs
}

func tailEvents(evs []SkelEvent, n int) []SkelEvent {
	if len(evs) <= n {
		return evs
	}
	return evs[len(evs)-n:]
}

// captureLog đọc phần đuôi của nhật ký và chỉ tổng hợp các tín hiệu cấu trúc (loại/lỗi/cảnh báo/stop_guard),
// Không đóng gói các dòng nhật ký thô - Chi tiết có thể chứa văn bản.
func captureLog(dir string, rc *RuntimeCapture) {
	path := filepath.Join(dir, "logs", "tui.log")
	tail, ok := readTail(path)
	if !ok {
		path = filepath.Join(dir, "logs", "headless.log")
		tail, ok = readTail(path)
	}
	if !ok {
		return
	}
	rc.Sources = append(rc.Sources, "logs/"+filepath.Base(path)+" (đuôi)")

	sc := bufio.NewScanner(bytes.NewReader(tail))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.Contains(line, "level=ERROR"):
			rc.LogErrors++
		case strings.Contains(line, "level=WARN"):
			rc.LogWarns++
		}
		if m := kindRe.FindStringSubmatch(line); m != nil {
			rc.LogKinds[m[1]]++
		}
		if strings.Contains(line, "stop_guard") {
			rc.StopGuard++
		}
	}
}

// readTail đọc byte logTailCap từ cuối tệp, loại bỏ nửa dòng đầu tiên có thể bị cắt bớt.
func readTail(path string) ([]byte, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false
	}
	size := info.Size()
	var off int64
	if size > logTailCap {
		off = size - logTailCap
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, false
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false
	}
	if off > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return data, true
}

func topRepeats(m map[string]int) []RepeatStat {
	var out []RepeatStat
	for sig, c := range m {
		if c >= repeatMin {
			out = append(out, RepeatStat{Sig: sig, Count: c})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Sig < out[j].Sig
	})
	if len(out) > repeatTopN {
		out = out[:repeatTopN]
	}
	return out
}

func topDups(m map[string]int) []DupStat {
	var out []DupStat
	for sha, c := range m {
		if c >= repeatMin {
			out = append(out, DupStat{Sha: sha, Count: c})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Sha < out[j].Sha
	})
	return out
}

func sortedModels(m map[string]RoleModel) []RoleModel {
	out := make([]RoleModel, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
	return out
}
