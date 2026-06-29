package host

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/bootstrap"
	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/models"
	storepkg "github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// nearSampleCap là kích thước cửa sổ trượt: chỉ N lệnh gọi gần đây nhất của mỗi vai trò (cacheRead, input) được giữ lại
// Mẫu, được sử dụng để so sánh tỷ lệ trúng "tích lũy so với gần N lần" ở cột bên trái và xác định "kéo sớm" so với "lượt truy cập thấp ở trạng thái ổn định".
const recentSampleCap = 10

// UsageTracker tích lũy mã thông báo đầu vào/đầu ra LLM và chi phí bằng đô la cho tất cả các đại lý trong suốt phiên.
//
// Cơ chế làm việc:
//   - Bản ghi cuộc gọi(agentName, msg) mỗi khi lệnh gọi lại OnMessage của tổng đài viên kích hoạt
//   - AgentName được ánh xạ tới vai trò (architect_* được chuẩn hóa thành Architect), kiểm tra mô hình hiện được liên kết với vai trò trong ModelSet
//   - Sử dụng models.DefaultRegistry để kiểm tra giá model và sử dụng phép nhân tích lũy bốn kỳ của đầu vào/đầu ra/đọc bộ đệm/ghi bộ đệm không phải bộ đệm
//   - Khi không có mô hình như vậy trong sổ đăng ký, msg.Usage.Cost.Total (nhà cung cấp đi kèm với nó, có thể là 0) sẽ được trả về.
//   - Chuyển nóng model (/model), các tin nhắn tiếp theo sẽ tự động định giá theo model mới, các tin nhắn cũ giữ nguyên giá cũ.
//
// Đồng thời duy trì kích thước của mỗi vai trò (nhà văn/biên tập viên/kiến trúc sư/điều phối viên):
//   - Dữ liệu lượt truy cập tích lũy → hiệu quả tối ưu hóa tổng thể
//   - Cửa sổ trượt N lần cuối → phân biệt lượt kéo sớm và lượt truy cập thấp ở trạng thái ổn định
//   - Cờ CacheCapable → phân biệt giữa "không bật" và "lượt truy cập thực sự 0%"
//
// Chủ đề an toàn.
type UsageTracker struct {
	mu       sync.Mutex
	overall  agentTotals
	perAgent map[string]*agentTotals // key là tên vai trò được chuẩn hóa của AgentRoleName
	perModel map[string]*agentTotals // Điều quan trọng là nhà cung cấp/mô hình; khi nhà cung cấp không xác định, nó sẽ thoái hóa thành mô hình
	modelSet *bootstrap.ModelSet
	store    *storepkg.Store // Có thể bằng 0 (kịch bản thử nghiệm), tất cả các phương thức lưu giữ đều im lặng khi không

	// missAssistantUsage Số tích lũy của "tin nhắn hỗ trợ đã nhận nhưng Mức sử dụng là không".
	// Theo các phép đo thực tế, điều này chủ yếu xảy ra khi phần phụ trợ tương thích OpenAI tự xây dựng không nhấn OpenAI khi kết thúc phát trực tuyến.
	// Khi giao thức streaming_options.include_usage gửi đoạn sử dụng cuối cùng - một phần.Usage
	// Luôn bằng 0, tất cả các trường tích lũy đều dừng ở 0. Bộ đếm cho phép giao diện người dùng trực tiếp thông báo cho người dùng "nó đang ngược dòng và sẽ không quay trở lại".
	// việc sử dụng không bị hỏng ở đây", thay vì bám vào mã bảng điều khiển bộ đệm.
	missingAssistantUsage int
	loggedMissingUsage    bool // Cả buổi chỉ cảnh báo 1 lần để tránh tui.log bị xóa.

	// saveCh được kích hoạt bởi Record theo cách không chặn sau khi tích lũy; autoSaveLoop lắng nghe và nhấn gỡ lỗi để giải phóng đĩa.
	// đệm=1: Nhiều Bản ghi liên tiếp được xếp thành một tín hiệu vị trí; khi đầy thì bỏ đi và ghi chung vào tích tắc tiếp theo.
	saveCh chan struct{}

	// onCost được gọi bên ngoài khóa với chi phí tích lũy mới nhất sau mỗi lần hạch toán (phát hiện đường chéo của BudgetSentinel).
	// Nó phải được đặt thông qua SetOnCost trước khi Bản ghi đồng thời bắt đầu và ở chế độ chỉ đọc sau đó.
	onCost func(total float64)

	// onMissingUsage được gọi một lần khi tìm thấy "thông báo trợ lý không sử dụng" lần đầu tiên (giống như cảnh báo slog
	// Máy đồng thời). Khi ngân sách được bật, điều này có nghĩa là điểm mù trong thanh toán - chi phí luôn bằng 0, ngân sách không bao giờ được kích hoạt và phải gọi cho ai đó.
	onMissingUsage func()
}

// useSample là mẫu lần truy cập của một OnMessage duy nhất và chỉ tử số và mẫu số của tỷ lệ lần truy cập được ghi lại.
type usageSample struct {
	CacheRead int
	Input     int
}

// AgentTotals là số lượng tích lũy của một đại lý.
//   - Đã lưu là phần chênh lệch "nếu tính theo giá không phải bộ đệm" được tính lại dựa trên dữ liệu lượt truy cập hiện tại
//   - CacheCapable chỉ được đặt thành true nếu vai trò đã được gọi ít nhất một lần bởi "mô hình có khả năng lưu trữ bộ đệm đã biết"
//   - mẫu là bộ đệm vòng có chiều dài cố định. Thời gian SampleCap gần đây đầu tiên được thêm trực tiếp và sau đó được xoay vòng theo sampleIdx.
type agentTotals struct {
	Input        int
	Output       int
	CacheRead    int
	CacheWrite   int
	Cost         float64
	Saved        float64
	CacheCapable bool
	samples      []usageSample
	sampleIdx    int
}

func NewUsageTracker(set *bootstrap.ModelSet, store *storepkg.Store) *UsageTracker {
	return &UsageTracker{
		modelSet: set,
		store:    store,
		perAgent: make(map[string]*agentTotals, 4),
		perModel: make(map[string]*agentTotals, 4),
		saveCh:   make(chan struct{}, 1),
	}
}

// Bản ghi phân phối thông báo tác nhân tới cả hai đường dẫn tích lũy/chẩn đoán.
//
// Việc tích lũy chỉ phụ thuộc vào việc Cách sử dụng có tồn tại hay không - "Thông báo nào mang Cách sử dụng" là bộ điều hợp Agentcore/litellm
// Chi tiết tập hợp (giao thức ngược dòng đặt mức sử dụng ở mức cao nhất của phản hồi), không cần thay đổi nếu quy tắc tập hợp thay đổi trong tương lai.
// Chẩn đoán yêu cầu Vai trò=Trợ lý và Nội dung không được trống để tránh AbortMsg / phục hồi ngoại lệ / công cụ /
// thông báo của người dùng gây ô nhiễm số lượng MissAssistantUsage.
func (t *UsageTracker) Record(agentName string, msg agentcore.AgentMessage) {
	if t == nil {
		return
	}
	m, ok := msg.(agentcore.Message)
	if !ok {
		return
	}
	if m.Usage == nil {
		if m.Role == agentcore.RoleAssistant && len(m.Content) > 0 {
			t.flagMissingUsage(agentName)
		}
		return
	}
	role := agentRoleName(agentName)
	provider, modelName := usageActualModel(m.Usage)
	t.accumulate(role, provider, modelName, *m.Usage)
}

func usageActualModel(u *agentcore.Usage) (provider, modelName string) {
	if u == nil {
		return "", ""
	}
	return strings.TrimSpace(u.Provider), strings.TrimSpace(u.Model)
}

// flagMissingUsage tích lũy một sự kiện "trông giống như phản hồi LLM thực nhưng không nhận được mức sử dụng" và chỉ truy cập nó một lần trong toàn bộ phiên.
// Nhật ký cảnh báo ngăn không cho tôi.log bị xóa.
func (t *UsageTracker) flagMissingUsage(agentName string) {
	t.mu.Lock()
	t.missingAssistantUsage++
	shouldLog := !t.loggedMissingUsage
	t.loggedMissingUsage = true
	t.mu.Unlock()
	if shouldLog {
		slog.Warn("Phản hồi LLM không mang dữ liệu sử dụng và bảng bộ đệm/chi phí sẽ không tích lũy - thông thường, luồng ngược dòng không gửi đoạn sử dụng cuối cùng theo giao thức OpenAI include_usage.",
			"module", "usage", "agent", agentName)
		if t.onMissingUsage != nil {
			t.onMissingUsage()
		}
	}
	t.notifyDirty()
}

// SetOnMissingUsage đăng ký lệnh gọi lại một lần cho "thiếu lần sử dụng đầu tiên".
// Phải được gọi một lần trong quá trình xây dựng Máy chủ và trước khi Bản ghi đồng thời bắt đầu.
func (t *UsageTracker) SetOnMissingUsage(cb func()) {
	if t == nil {
		return
	}
	t.onMissingUsage = cb
}

// Việc không chặn thông báoDirty sẽ kích hoạt tín hiệu giảm, tín hiệu này thực sự được viết bởi autoSaveLoop theo bản gỡ lỗi.
// Kênh tín hiệu được đệm=1: Nhiều yêu cầu Ghi liên tiếp có thể được xếp thành một yêu cầu lưu.
func (t *UsageTracker) notifyDirty() {
	if t == nil || t.saveCh == nil {
		return
	}
	select {
	case t.saveCh <- struct{}{}:
	default:
	}
}

// tích lũy Tích lũy một thông báo với Mức sử dụng thành ba số lượng: tổng thể / mỗi vai trò / mỗi mô hình.
// Nếu nhà cung cấp/mô hình trống, điều đó có nghĩa là "sử dụng ModelSet hiện tại để lấy mô hình tương ứng với vai trò" (đường dẫn thời gian thực); nếu nó không trống, điều đó có nghĩa là
// "Buộc định giá dựa trên mô hình đã chỉ định" (đường dẫn phát lại sử dụng _meta trong phiên jsonl).
// ResolveCost được thực thi bên ngoài khóa (nó chỉ đọc modelSet/Registry) và chỉ việc bổ sung được thực hiện bên trong khóa.
func (t *UsageTracker) accumulate(role, provider, modelName string, u agentcore.Usage) {
	provider, modelName = t.effectiveModel(role, provider, modelName)
	cost, saved, capable := t.resolveCost(modelName, u)

	t.mu.Lock()
	addUsage(&t.overall, u, cost, saved, capable)

	per := t.perAgent[role]
	if per == nil {
		per = &agentTotals{}
		t.perAgent[role] = per
	}
	addUsage(per, u, cost, saved, capable)

	if key := modelUsageKey(provider, modelName); key != "" {
		perModel := t.perModel[key]
		if perModel == nil {
			perModel = &agentTotals{}
			t.perModel[key] = perModel
		}
		addUsage(perModel, u, cost, saved, capable)
	}
	total := t.overall.Cost
	t.mu.Unlock()

	t.notifyDirty()
	if t.onCost != nil {
		t.onCost(total)
	}
}

// SetOnCost đăng ký lệnh gọi lại kế toán (mang chi phí tích lũy mới nhất, được gọi bên ngoài khóa).
// Phải được gọi một lần trong quá trình xây dựng Máy chủ và trước khi Bản ghi đồng thời bắt đầu.
func (t *UsageTracker) SetOnCost(cb func(total float64)) {
	if t == nil {
		return
	}
	t.onCost = cb
}

func (t *UsageTracker) effectiveModel(role, provider, modelName string) (string, string) {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		if t != nil && t.modelSet != nil {
			p, m, _ := t.modelSet.CurrentSelection(role)
			return p, m
		}
		return "", ""
	}
	if provider == "" && t != nil && t.modelSet != nil {
		p, m, _ := t.modelSet.CurrentSelection(role)
		if m == modelName {
			provider = p
		}
	}
	return provider, modelName
}

func modelUsageKey(provider, modelName string) string {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	switch {
	case modelName == "":
		return ""
	case provider == "":
		return modelName
	default:
		return provider + "/" + modelName
	}
}

// addUsage thêm mã thông báo và chi phí của một cuộc gọi vào tổng số.
// Phải được gọi với UsageTracker.mu trong tay.
//
// CacheCapable trước tiên được xác định bằng "sự thật": miễn là nó thấy CacheRead hoặc CacheWrite > 0 là chứng minh
// Ngược dòng thực hiện lưu vào bộ nhớ đệm nhanh chóng. CacheReadCostPer1M của cơ quan đăng ký chỉ dành cho dự phòng.
// Bởi vì các mô hình backend tự xây dựng (mimo-v2.5-pro/đại lý trong nước, v.v.) thường không có sẵn trong BerriAI/litellm
// chỉ số giá, nhưng thực tế có dữ liệu bộ đệm trong phần Cách sử dụng, vì vậy giao diện người dùng không nên đánh giá sai dữ liệu đó là "không được bật".
func addUsage(t *agentTotals, u agentcore.Usage, cost, saved float64, capable bool) {
	t.Input += u.Input
	t.Output += u.Output
	t.CacheRead += u.CacheRead
	t.CacheWrite += u.CacheWrite
	t.Cost += cost
	t.Saved += saved
	if capable || u.CacheRead > 0 || u.CacheWrite > 0 {
		t.CacheCapable = true
	}
	pushSample(t, u.CacheRead, u.Input)
}

// pushSample đẩy một mẫu vào bộ đệm vòng. Gần đâySampleCap đầu tiên là phần bổ sung thuần túy và sau đó được bao phủ bởi phép xoay.
func pushSample(t *agentTotals, cacheRead, input int) {
	s := usageSample{CacheRead: cacheRead, Input: input}
	if len(t.samples) < recentSampleCap {
		t.samples = append(t.samples, s)
		return
	}
	t.samples[t.sampleIdx] = s
	t.sampleIdx = (t.sampleIdx + 1) % recentSampleCap
}

// gần đâySums trả về tổng của cacheRead và dữ liệu đầu vào trong cửa sổ trượt, dưới dạng tử số và mẫu số của "tốc độ truy cập gần N".
// Sử dụng tổng/tổng ​​thay vì "trung bình của các tỷ lệ đơn" để tránh khuếch đại nhiễu với các mẫu nhỏ (đầu vào=hàng trăm mã thông báo).
func recentSums(t *agentTotals) (cacheRead, input int) {
	for _, s := range t.samples {
		cacheRead += s.CacheRead
		input += s.Input
	}
	return cacheRead, input
}

// Tổng số trả về ảnh chụp nhanh của tổng số tích lũy.
func (t *UsageTracker) Totals() (cost float64, input, output, cacheRead, cacheWrite int) {
	if t == nil {
		return 0, 0, 0, 0, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.overall.Cost, t.overall.Input, t.overall.Output, t.overall.CacheRead, t.overall.CacheWrite
}

// Đã lưuUSD Trả về số đô la tích lũy được lưu do lần truy cập bộ đệm.
func (t *UsageTracker) SavedUSD() float64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.overall.Saved
}

// Tổng thểRecent trả về tổng số lần đọc bộ đệm, tổng số đầu vào và số lượng mẫu trong cửa sổ trượt (< thời gian SampleCap gần đây).
func (t *UsageTracker) OverallRecent() (cacheRead, input, samples int) {
	if t == nil {
		return 0, 0, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	r, in := recentSums(&t.overall)
	return r, in, len(t.overall.samples)
}

// Tổng thểCacheCapable Cho dù toàn bộ đã được thông qua một mô hình được biết là hỗ trợ bộ đệm ít nhất một lần.
func (t *UsageTracker) OverallCacheCapable() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.overall.CacheCapable
}

// MissingAssistantUsage trả về số lượng tích lũy của "tin nhắn hỗ trợ đã nhận nhưng Mức sử dụng là không".
// Lớn hơn 0 thường có nghĩa là luồng ngược dòng không gửi đoạn sử dụng cuối cùng của OpenAI.
// Giao diện người dùng hiển thị lời nhắc tương ứng thay vì nhầm tưởng rằng mô-đun bộ đệm đã bị hỏng.
func (t *UsageTracker) MissingAssistantUsage() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.missingAssistantUsage
}

// ── Kiên trì ──

// Trạng thái tích lũy hiện tại của bản sao chụp nhanh là miền có thể tuần tự hóa.UsageState.
// Các mẫu cửa sổ trượt không nhập ảnh chụp nhanh - đây là cửa sổ chẩn đoán ngắn hạn và ít có ý nghĩa trong các quy trình.
func (t *UsageTracker) Snapshot() domain.UsageState {
	if t == nil {
		return domain.UsageState{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	state := domain.UsageState{
		Schema:       domain.UsageSchemaVersion,
		UpdatedAt:    time.Now(),
		Overall:      totalsSnapshot(&t.overall),
		PerAgent:     make(map[string]domain.AgentUsageTotals, len(t.perAgent)),
		PerModel:     make(map[string]domain.AgentUsageTotals, len(t.perModel)),
		MissingUsage: t.missingAssistantUsage,
	}
	for role, v := range t.perAgent {
		state.PerAgent[role] = totalsSnapshot(v)
	}
	for model, v := range t.perModel {
		state.PerModel[model] = totalsSnapshot(v)
	}
	return state
}

// LoadFromStore đọc ảnh chụp nhanh liên tục từ store.Usage và chèn nó vào bộ nhớ. Trả lại phương tiện thực sự
// Đã tải thành công vào trạng thái không trống (khớp lược đồ); sai có nghĩa là không có tập tin hoặc không có sẵn, người gọi
// Bạn nên tiếp tục sử dụng tính năng phát lại phiên để chèn lấp một lần.
func (t *UsageTracker) LoadFromStore() (bool, error) {
	if t == nil || t.store == nil {
		return false, nil
	}
	state, err := t.store.Usage.Load()
	if err != nil {
		return false, err
	}
	if state == nil {
		return false, nil
	}
	t.applyState(*state)
	return true, nil
}

// SaveNow ngay lập tức lưu ảnh chụp nhanh hiện tại vào đĩa. autoSaveLoop/Đóng đường dẫn được viết thông qua nó.
func (t *UsageTracker) SaveNow() error {
	if t == nil || t.store == nil {
		return nil
	}
	return t.store.Usage.Save(t.Snapshot())
}

// StartAutoSave khởi động một goroutine và giám sát việc gỡ lỗi saveCh +. ctx xong trước cuộc họp
// Xóa trạng thái chưa được lưu cuối cùng. Đóng kích hoạt tuôn ra + thoát qua hủy ctx.
func (t *UsageTracker) StartAutoSave(ctx context.Context) {
	if t == nil || t.store == nil {
		return
	}
	go t.autoSaveLoop(ctx)
}

// autoSaveLoop điều chỉnh tín hiệu bẩn tần số cao xuống đĩa sau mỗi 500 mili giây.
//
// Lưu ý thiết kế: 500ms là giá trị kinh nghiệm - 1-2 lượt LLM mỗi chương, 1-2 vị trí là hoàn toàn có thể chấp nhận được;
// Ngay cả khi người dùng thoát thủ công bằng ctrl+C trước khi kích hoạt bộ hẹn giờ, đường dẫn hủy ctx sẽ bị xóa lần cuối.
// Một sự cố thực sự (OS kill -9) sẽ mất tích lũy trong vòng 0,5 giây cuối cùng - phiên ngược dòng jsonl sẽ vẫn
// Để biết thông tin đầy đủ, sự khác biệt sẽ được khắc phục từ các phiên/phát lại vào lần khởi động tiếp theo.
func (t *UsageTracker) autoSaveLoop(ctx context.Context) {
	const debounce = 500 * time.Millisecond
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	var pending bool
	flush := func() {
		if err := t.SaveNow(); err != nil {
			slog.Warn("việc sử dụng không thể đặt đĩa", "module", "usage", "err", err)
		}
		pending = false
	}
	for {
		select {
		case <-ctx.Done():
			if pending {
				flush()
			}
			return
		case <-t.saveCh:
			if pending {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			timer.Reset(debounce)
			pending = true
		case <-timer.C:
			flush()
		}
	}
}

// applyState ghi ảnh chụp nhanh liên tục vào bộ nhớ. Chỉ được gọi khi khởi động (sau LoadFromStore/replay),
// Tại thời điểm này, autoSaveLoop/Record chưa được khởi động và sẽ không được kích hoạt đồng thời nên không cần khóa; nhưng mu được giữ lại trong trường hợp
// Việc kiểm tra hoặc những thay đổi trong tương lai về thứ tự cuộc gọi sẽ đưa đến tính đồng thời.
func (t *UsageTracker) applyState(state domain.UsageState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.overall = totalsFromState(state.Overall)
	if state.PerAgent == nil {
		t.perAgent = make(map[string]*agentTotals, 4)
	} else {
		t.perAgent = make(map[string]*agentTotals, len(state.PerAgent))
		for role, v := range state.PerAgent {
			tot := totalsFromState(v)
			t.perAgent[role] = &tot
		}
	}
	if state.PerModel == nil {
		t.perModel = make(map[string]*agentTotals, 4)
	} else {
		t.perModel = make(map[string]*agentTotals, len(state.PerModel))
		for model, v := range state.PerModel {
			tot := totalsFromState(v)
			t.perModel[model] = &tot
		}
	}
	t.missingAssistantUsage = state.MissingUsage
}

// TotalsSnapshot sao chép tác nhânTotals trong bộ nhớ vào miền cố định.AgentUsageTotals.
// Bộ đệm vòng mẫu không được cố ý lấy ra - xem chú thích UsageState.
func totalsSnapshot(t *agentTotals) domain.AgentUsageTotals {
	if t == nil {
		return domain.AgentUsageTotals{}
	}
	return domain.AgentUsageTotals{
		Input:        t.Input,
		Output:       t.Output,
		CacheRead:    t.CacheRead,
		CacheWrite:   t.CacheWrite,
		Cost:         t.Cost,
		Saved:        t.Saved,
		CacheCapable: t.CacheCapable,
	}
}

// TotalsFromState khôi phục trạng thái liên tục thành tác nhânTotals trong bộ nhớ. Để trống các mẫu sau khi khởi động lại
// Quá trình tích lũy lại bắt đầu từ 0 và ngữ nghĩa "tỷ lệ trúng gần N" có thể được khôi phục sau vài vòng Ghi.
func totalsFromState(s domain.AgentUsageTotals) agentTotals {
	return agentTotals{
		Input:        s.Input,
		Output:       s.Output,
		CacheRead:    s.CacheRead,
		CacheWrite:   s.CacheWrite,
		Cost:         s.Cost,
		Saved:        s.Saved,
		CacheCapable: s.CacheCapable,
	}
}

// AgentUsage là ảnh chụp nhanh mức sử dụng tích lũy của tác nhân (được hiển thị trên giao diện người dùng).
type AgentUsage struct {
	Role            string
	Model           string
	Input           int
	Output          int
	CacheRead       int
	CacheWrite      int
	Cost            float64
	Saved           float64
	CacheCapable    bool
	RecentCacheRead int
	RecentInput     int
	RecentSamples   int
}

// PerAgent trả về mức sử dụng tích lũy của từng vai trò. Các kết quả được sắp xếp theo thứ tự giảm dần theo số lượng CacheRead và các vai trò chưa sử dụng mã thông báo sẽ bị bỏ qua.
func (t *UsageTracker) PerAgent() []AgentUsage {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]AgentUsage, 0, len(t.perAgent))
	for role, v := range t.perAgent {
		if v.Input == 0 && v.Output == 0 {
			continue
		}
		recentRead, recentInput := recentSums(v)
		out = append(out, AgentUsage{
			Role:            role,
			Input:           v.Input,
			Output:          v.Output,
			CacheRead:       v.CacheRead,
			CacheWrite:      v.CacheWrite,
			Cost:            v.Cost,
			Saved:           v.Saved,
			CacheCapable:    v.CacheCapable,
			RecentCacheRead: recentRead,
			RecentInput:     recentInput,
			RecentSamples:   len(v.samples),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CacheRead != out[j].CacheRead {
			return out[i].CacheRead > out[j].CacheRead
		}
		return out[i].Input > out[j].Input
	})
	return out
}

// PerModel trả về mức sử dụng tích lũy của từng mô hình. Các kết quả được sắp xếp giảm dần theo chi phí, theo sau là thứ tự giảm dần theo khối lượng đầu vào.
func (t *UsageTracker) PerModel() []AgentUsage {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]AgentUsage, 0, len(t.perModel))
	for model, v := range t.perModel {
		if v.Input == 0 && v.Output == 0 {
			continue
		}
		out = append(out, AgentUsage{
			Model:        model,
			Input:        v.Input,
			Output:       v.Output,
			CacheRead:    v.CacheRead,
			CacheWrite:   v.CacheWrite,
			Cost:         v.Cost,
			Saved:        v.Saved,
			CacheCapable: v.CacheCapable,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Input > out[j].Input
	})
	return out
}

// ResolveCost cũng trả về chi phí/đã lưu/có khả năng của thông báo này.
//   - chi phí: Số lần đăng ký được nhân với 4 mục; sự bỏ lỡ sẽ bị loại bỏ khi nhà cung cấp phải trả chi phí riêng
//   - đã lưu: chỉ các lần đăng ký, CacheRead > 0 và inputCost > CacheReadCost > 0
//   - có khả năng: nhấn đăng ký và mô hình CacheReadCostPer1M > 0 → được biết là hỗ trợ bộ nhớ đệm nhanh chóng
//
// ModelName được người gọi truyền vào được ưu tiên (_meta.model từ phiên jsonl trong khi phát lại).
func (t *UsageTracker) resolveCost(modelName string, u agentcore.Usage) (cost, saved float64, capable bool) {
	if entry, ok := models.DefaultRegistry().Resolve(modelName); ok {
		c := computeCost(u, *entry)
		s := computeSaved(u, *entry)
		canCache := entry.CacheReadCostPer1M > 0
		if c > 0 {
			return c, s, canCache
		}
	}
	if u.Cost != nil {
		return u.Cost.Total, 0, false
	}
	return 0, 0, false
}

// AgentRoleName bình thường hóa tên tác nhân phụ thành tên vai trò.
// Architect_short/mid/long đều được gán cho Architect; những người khác được trả lại không thay đổi.
func agentRoleName(agentName string) string {
	if strings.HasPrefix(agentName, "architect_") {
		return "architect"
	}
	return agentName
}

// tính toánCost tính toán chi phí bằng đô la của lệnh gọi này dựa trên đơn giá của mã thông báo $/1 triệu đô la.
//
// Tiền đề ngữ nghĩa (được đảm bảo bởi từng nhà cung cấp litellm một cách thống nhất, xem anthropic.go/bedrock.go/
// Điểm lắp ráp sử dụng cho openai.go/gemini.go/compat.go):
//
//	u.Input = Tất cả các mã thông báo đầu vào, **bao gồm** CacheRead; không bao gồm CacheWrite
//	u.Output = mã thông báo đầu ra
//
// Do đó nonCachedInput = u.Input - u.CacheRead giữ cho tất cả các nhà cung cấp.
// Mục đích của việc giữ lại nhánh dưới cùng là để ngăn nó bị hỏng khi nhà cung cấp vô tình trả về dữ liệu bẩn trong tương lai.
func computeCost(u agentcore.Usage, e models.ModelEntry) float64 {
	nonCachedInput := u.Input - u.CacheRead
	if nonCachedInput < 0 {
		nonCachedInput = u.Input
	}
	c := 0.0
	c += float64(nonCachedInput) * e.InputCostPer1M / 1_000_000
	c += float64(u.Output) * e.OutputCostPer1M / 1_000_000
	c += float64(u.CacheRead) * e.CacheReadCostPer1M / 1_000_000
	c += float64(u.CacheWrite) * e.CacheWriteCostPer1M / 1_000_000
	return c
}

// tínhSaved ước tính số tiền tiết kiệm được từ các lần truy cập CacheRead so với "được thanh toán theo giá đầu vào thông thường".
// Lưu ý rằng phí bảo hiểm của CacheWrite không được khấu trừ - đây là khoản đầu tư cần thiết để "mở đường cho những lần truy cập tiếp theo".
// Thu nhập thực tế được thu hồi tích lũy từ các lần CacheRead tiếp theo.
func computeSaved(u agentcore.Usage, e models.ModelEntry) float64 {
	if u.CacheRead <= 0 || e.InputCostPer1M <= 0 {
		return 0
	}
	delta := e.InputCostPer1M - e.CacheReadCostPer1M
	if delta <= 0 {
		return 0
	}
	return float64(u.CacheRead) * delta / 1_000_000
}
