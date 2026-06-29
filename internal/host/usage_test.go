package host

import (
	"testing"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/models"
)

// makeUsageMsg Xây dựng một thông báo (có Cách sử dụng) mà lệnh gọi lại OnMessage có thể chấp nhận.
// Vai trò phải được đặt rõ ràng thành trợ lý: UsageTracker.Record Bây giờ lọc theo vai trò,
// Chỉ các tin nhắn trợ lý mới được tích lũy (các vai trò khác đương nhiên không có quyền sử dụng).
func makeUsageMsg(input, cacheRead, cacheWrite, output int) agentcore.AgentMessage {
	return agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Input: input, Output: output, CacheRead: cacheRead, CacheWrite: cacheWrite,
		},
	}
}

// Test_pushSample_RingBuffer xác minh ngữ nghĩa xoay của cửa sổ trượt:
// N lần đầu tiên được thêm trực tiếp; sau đó, mục nhập cũ nhất sẽ bị ghi đè bởi sampleIdx. các tổng gần đây luôn phản ánh "N lần cuối cùng".
func Test_pushSample_RingBuffer(t *testing.T) {
	var tot agentTotals

	for i := 1; i <= recentSampleCap; i++ {
		pushSample(&tot, i, i*100)
	}
	if got := len(tot.samples); got != recentSampleCap {
		t.Fatalf("after %d pushes, samples len=%d want %d", recentSampleCap, got, recentSampleCap)
	}

	pushSample(&tot, 999, 99900)
	if got := len(tot.samples); got != recentSampleCap {
		t.Fatalf("after overflow, samples len=%d want %d (no growth)", got, recentSampleCap)
	}
	cacheRead, input := recentSums(&tot)
	expectedCacheRead := 999
	expectedInput := 99900
	for i := 2; i <= recentSampleCap; i++ {
		expectedCacheRead += i
		expectedInput += i * 100
	}
	if cacheRead != expectedCacheRead || input != expectedInput {
		t.Fatalf("recentSums after overflow = (%d, %d), want (%d, %d)",
			cacheRead, input, expectedCacheRead, expectedInput)
	}
}

// Test_UsageTracker_RecordAccumulates xác minh rằng Ghi nhiều vai trò được tích lũy chính xác.
// Sáp nhập tổng thể = tổng của tất cả các vai trò; mỗi vai trò là độc lập.
func Test_UsageTracker_RecordAccumulates(t *testing.T) {
	tk := NewUsageTracker(nil, nil) // modelSet=nil → Tận dụng Chi phí của nhà cung cấp mà không ảnh hưởng đến logic tích lũy.

	tk.Record("writer", makeUsageMsg(1000, 800, 0, 200))
	tk.Record("writer", makeUsageMsg(1500, 1200, 100, 300))
	tk.Record("editor", makeUsageMsg(500, 0, 0, 100))

	cost, in, out, cr, cw := tk.Totals()
	if in != 3000 || out != 600 || cr != 2000 || cw != 100 {
		t.Fatalf("totals = (in=%d out=%d cr=%d cw=%d), want (3000 600 2000 100)", in, out, cr, cw)
	}
	if cost != 0 {
		t.Errorf("cost should be 0 when modelSet=nil and no provider Cost, got %v", cost)
	}

	per := tk.PerAgent()
	if len(per) != 2 {
		t.Fatalf("per-agent len=%d want 2", len(per))
	}
	// PerAgent theo thứ tự giảm dần của CacheRead: writer (2000) nên đứng trước editor (0)
	if per[0].Role != "writer" || per[1].Role != "editor" {
		t.Fatalf("per-agent order = %s,%s want writer,editor", per[0].Role, per[1].Role)
	}
	if per[0].Input != 2500 || per[0].CacheRead != 2000 {
		t.Errorf("writer totals = (in=%d cr=%d), want (2500 2000)", per[0].Input, per[0].CacheRead)
	}
}

// Test_UsageTracker_ArchitectAliasNormalized Verification architecture_short/mid/long
// Tất cả đều được thống nhất về cùng một khóa "kiến trúc sư" (để tránh bị chia thành nhiều dòng bởi các vai trò phụ được chuyển đổi bởi /model).
func Test_UsageTracker_ArchitectAliasNormalized(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	tk.Record("architect_short", makeUsageMsg(100, 50, 0, 20))
	tk.Record("architect_mid", makeUsageMsg(200, 100, 0, 40))
	tk.Record("architect_long", makeUsageMsg(300, 150, 0, 60))

	per := tk.PerAgent()
	if len(per) != 1 {
		t.Fatalf("aliases must merge to single role, got %d entries: %+v", len(per), per)
	}
	if per[0].Role != "architect" {
		t.Fatalf("merged role name = %q, want architect", per[0].Role)
	}
	if per[0].Input != 600 || per[0].CacheRead != 300 {
		t.Errorf("merged totals = (in=%d cr=%d), want (600 300)", per[0].Input, per[0].CacheRead)
	}
}

func Test_UsageTracker_PerModelAccumulates(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	tk.accumulate("writer", "openrouter", "model-a", agentcore.Usage{Input: 1000, Output: 200, CacheRead: 700})
	tk.accumulate("editor", "openrouter", "model-b", agentcore.Usage{Input: 500, Output: 100})
	tk.accumulate("writer", "openrouter", "model-a", agentcore.Usage{Input: 300, Output: 80, CacheRead: 200})

	perModel := tk.PerModel()
	if len(perModel) != 2 {
		t.Fatalf("per-model len=%d want 2", len(perModel))
	}
	seen := map[string]AgentUsage{}
	for _, m := range perModel {
		seen[m.Model] = m
	}
	if seen["openrouter/model-a"].Input != 1300 || seen["openrouter/model-a"].CacheRead != 900 {
		t.Errorf("model-a totals = %+v", seen["openrouter/model-a"])
	}
	if seen["openrouter/model-b"].Output != 100 {
		t.Errorf("model-b totals = %+v", seen["openrouter/model-b"])
	}

	snap := tk.Snapshot()
	restored := NewUsageTracker(nil, nil)
	restored.applyState(snap)
	if got := restored.PerModel(); len(got) != 2 {
		t.Fatalf("restored per-model len=%d want 2: %+v", len(got), got)
	}
}

func Test_UsageTracker_RecordUsesActualUsageModel(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	tk.Record("writer", agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Provider: "openrouter",
			Model:    "google/gemini-2.5-pro",
			Input:    1000,
			Output:   200,
		},
	})

	perModel := tk.PerModel()
	if len(perModel) != 1 {
		t.Fatalf("per-model len=%d want 1: %+v", len(perModel), perModel)
	}
	if perModel[0].Model != "openrouter/google/gemini-2.5-pro" {
		t.Fatalf("model key = %q, want openrouter/google/gemini-2.5-pro", perModel[0].Model)
	}
	if perModel[0].Input != 1000 || perModel[0].Output != 200 {
		t.Fatalf("model totals = %+v", perModel[0])
	}
}

func Test_UsageTracker_ProviderOnlyDoesNotInventModelKey(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	tk.Record("writer", agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Provider: "openrouter",
			Input:    1000,
			Output:   200,
		},
	})

	if got := tk.PerModel(); len(got) != 0 {
		t.Fatalf("provider-only usage must not create model stats without a model, got %+v", got)
	}
}

// Test_UsageTracker_RecentWindowReflectsLatest xác minh rằng cửa sổ trượt phản ánh "N lần cuối cùng",
// Không bị kéo xuống bởi lượt truy cập thấp sớm - Đây chính xác là vấn đề "kéo sớm so với lượt truy cập thấp ở trạng thái ổn định" mà P1 muốn giải quyết.
func Test_UsageTracker_RecentWindowReflectsLatest(t *testing.T) {
	tk := NewUsageTracker(nil, nil)

	// 5 lượt truy cập đầu tiên rất thấp (cảnh chương đầu tiên)
	for i := 0; i < 5; i++ {
		tk.Record("writer", makeUsageMsg(1000, 0, 0, 200))
	}
	// 8 (>5) lượt truy cập cao gần đây nhất (kịch bản trạng thái ổn định)
	for i := 0; i < 8; i++ {
		tk.Record("writer", makeUsageMsg(1000, 900, 0, 200))
	}

	per := tk.PerAgent()
	if len(per) != 1 {
		t.Fatalf("len=%d want 1", len(per))
	}
	w := per[0]

	// Tích lũy: 8 trên 13 lượt truy cập → 7200/13000 ≈ 55,4%
	cumulativeRate := float64(w.CacheRead) / float64(w.Input) * 100
	if cumulativeRate < 50 || cumulativeRate > 60 {
		t.Errorf("cumulative hit rate = %.1f%%, want ~55%%", cumulativeRate)
	}

	// Cửa sổ trượt: 8 lượt truy cập cao + 2 lượt truy cập 0 trong 10 lần gần nhất → 7200/10000 = 72%
	if w.RecentSamples != recentSampleCap {
		t.Errorf("recent samples = %d, want %d (window full)", w.RecentSamples, recentSampleCap)
	}
	recentRate := float64(w.RecentCacheRead) / float64(w.RecentInput) * 100
	if recentRate < 70 || recentRate > 75 {
		t.Errorf("recent hit rate = %.1f%%, want ~72%% (proves window dropped early misses)", recentRate)
	}
	// Key: N lần cuối cùng cao hơn đáng kể so với giá trị tích lũy, chứng tỏ số 0 sớm đã bị ném ra ngoài cửa sổ
	if recentRate <= cumulativeRate {
		t.Errorf("recent (%.1f%%) must exceed cumulative (%.1f%%) once window slides past early misses",
			recentRate, cumulativeRate)
	}
}

// Test_computeSaved xác minh thuật toán đã lưu: CacheRead × (Giá đầu vào - Giá CacheRead);
// Trả về 0 khi chênh lệch 0 hoặc Chi phí đầu vào 0 (Phí bảo hiểm CacheWrite không được khấu trừ).
func Test_computeSaved(t *testing.T) {
	cases := []struct {
		name  string
		usage agentcore.Usage
		entry models.ModelEntry
		want  float64
	}{
		{
			name:  "nhân bản 5m đánh cứu 90%",
			usage: agentcore.Usage{Input: 100_000, CacheRead: 80_000},
			entry: models.ModelEntry{InputCostPer1M: 3.0, CacheReadCostPer1M: 0.3},
			want:  80_000 * (3.0 - 0.3) / 1_000_000, // 0.216
		},
		{
			name:  "Không có lượt truy cập nào được lưu=0",
			usage: agentcore.Usage{Input: 100_000, CacheRead: 0},
			entry: models.ModelEntry{InputCostPer1M: 3.0, CacheReadCostPer1M: 0.3},
			want:  0,
		},
		{
			name:  "Model chưa được lưu giá=0",
			usage: agentcore.Usage{Input: 100_000, CacheRead: 50_000},
			entry: models.ModelEntry{InputCostPer1M: 0, CacheReadCostPer1M: 0},
			want:  0,
		},
		{
			name:  "Đã lưu mức chênh lệch bất thường=0",
			usage: agentcore.Usage{Input: 100_000, CacheRead: 50_000},
			entry: models.ModelEntry{InputCostPer1M: 1.0, CacheReadCostPer1M: 2.0}, // Bộ nhớ đệm đắt hơn
			want:  0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeSaved(tc.usage, tc.entry)
			if got != tc.want {
				t.Errorf("computeSaved=%v want %v", got, tc.want)
			}
		})
	}
}

// Test_UsageTracker_CacheCapableSticky xác minh rằng CacheCapable không quay trở lại sau khi được đặt thành true.
// Trước đây, các mô hình hỗ trợ bộ đệm đã được chạy → dữ liệu lần truy cập tích lũy là hợp lệ; việc chuyển sang một mô hình không được hỗ trợ giữa chừng sẽ không khiến nhãn hiệu bị khôi phục.
//
// Trực tiếp chỉ định mô phỏng bằng cách xây dựng perAgent (đường dẫn giải quyếtCost yêu cầu ModelSet+Registry, lớp tích hợp đã được đề cập).
func Test_UsageTracker_CacheCapableSticky(t *testing.T) {
	tk := NewUsageTracker(nil, nil)

	// Mô phỏng "đã từng chạy mô hình hỗ trợ bộ đệm + lần truy cập"
	tk.perAgent["writer"] = &agentTotals{
		Input: 1000, CacheRead: 500, Output: 200, CacheCapable: true,
	}
	// Sau đó, thêm "cuộc gọi mô hình không hỗ trợ bộ đệm"
	tk.Record("writer", makeUsageMsg(500, 0, 0, 100))

	per := tk.PerAgent()
	if len(per) != 1 || per[0].Role != "writer" {
		t.Fatalf("expected single writer entry, got %+v", per)
	}
	if !per[0].CacheCapable {
		t.Errorf("CacheCapable must remain true after switching to non-capable model")
	}
	if per[0].CacheRead != 500 || per[0].Input != 1500 {
		t.Errorf("totals after merge = (in=%d cr=%d), want (1500 500)",
			per[0].Input, per[0].CacheRead)
	}
}

// Test_UsageTracker_PerAgentSkipsZero xác minh rằng các vai trò có mã thông báo chưa sử dụng không xuất hiện trong PerAgent.
func Test_UsageTracker_PerAgentSkipsZero(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	// Xây dựng vai trò nhưng không tiêu thụ mã thông báo (trường hợp cực đoan)
	tk.perAgent["ghost"] = &agentTotals{}
	tk.Record("writer", makeUsageMsg(100, 50, 0, 20))

	per := tk.PerAgent()
	if len(per) != 1 || per[0].Role != "writer" {
		t.Fatalf("ghost role with zero tokens must be skipped, got %+v", per)
	}
}

// Test_UsageTracker_MissingAssistantUsageCounted Thiếu xác thựcAssistantUsage
// Giới hạn quyết định để tính:
//   - Đường dẫn tích lũy chỉ nhìn vào Usage != nil (không gắn với Role)
//   - Đường dẫn chẩn đoán yêu cầu Vai trò=Trợ lý và Nội dung không được trống - đây giống như "phản hồi LLM thực sự nhưng
//     Không nhận được "Usage", tương ứng với luồng ngược dòng không gửi bản cuối cùng OpenAI include_usage
//     Hành vi điển hình của chunk. Các tình huống khác (tin nhắn của người dùng/công cụ, trợ lý có nội dung trống)
//     Không ai trong số họ bị thiếu.
func Test_UsageTracker_MissingAssistantUsageCounted(t *testing.T) {
	tk := NewUsageTracker(nil, nil)

	withContent := func(text string) agentcore.Message {
		return agentcore.Message{
			Role:    agentcore.RoleAssistant,
			Content: []agentcore.ContentBlock{agentcore.TextBlock(text)},
		}
	}

	// trợ lý + có Nội dung + không Cách sử dụng → có vẻ là phản hồi thực sự nhưng thiếu cách sử dụng, được đưa vào chẩn đoán
	tk.Record("writer", withContent("hi"))
	tk.Record("writer", withContent("again"))
	// trợ lý nhưng Nội dung trống → đường dẫn khôi phục ngoại lệ hoặc thông báo giữ chỗ, không thiếu
	tk.Record("writer", agentcore.Message{Role: agentcore.RoleAssistant})
	// Thông báo người dùng/công cụ đương nhiên không mang theo cách sử dụng và nó không được tính là thiếu bất kể Nội dung có trống hay không.
	tk.Record("writer", agentcore.Message{Role: agentcore.RoleUser, Content: []agentcore.ContentBlock{agentcore.TextBlock("u")}})
	tk.Record("writer", agentcore.Message{Role: agentcore.RoleTool, Content: []agentcore.ContentBlock{agentcore.TextBlock("t")}})
	// Cách sử dụng thông thường → Đi đường tích lũy, không đưa vào chẩn đoán
	tk.Record("writer", makeUsageMsg(100, 50, 0, 20))

	if got := tk.MissingAssistantUsage(); got != 2 {
		t.Errorf("MissingAssistantUsage=%d, want 2", got)
	}
	_, in, _, _, _ := tk.Totals()
	if in != 100 {
		t.Errorf("Đường dẫn bình thường bị hủy tích lũy, input=%d muốn 100", in)
	}
}

// Test_UsageTracker_CacheCapableFromFacts xác minh CacheCapable khi không thể tìm thấy mô hình trong sổ đăng ký
// Vẫn có thể được đánh dấu là đúng dựa trên "thực tế": các mô hình phụ trợ proxy tự xây dựng/trong nước thường không có trong BerriAI/litellm
// Trong chỉ mục định giá của , ResolveCost trả về khả năng=false; nhưng miễn là chương trình phụ trợ thực sự có lợi
// CacheRead hoặc CacheWrite > 0, chứng tỏ mô hình hỗ trợ một cách khách quan các dòng nhắc nhở, dòng theo vai trò
// Nó sẽ không hiển thị "Chưa kích hoạt".
func Test_UsageTracker_CacheCapableFromFacts(t *testing.T) {
	tk := NewUsageTracker(nil, nil) // modelSet=nil → ResolveCost luôn có khả năng=false

	// Khi có CacheWrite (mô phỏng lần ghi đầu tiên vào bộ đệm, sổ đăng ký không được đánh dấu là có khả năng, nhưng hóa ra nó hỗ trợ nó)
	tk.Record("writer", makeUsageMsg(1000, 0, 200, 100))
	per := tk.PerAgent()
	if len(per) != 1 || !per[0].CacheCapable {
		t.Fatalf("CacheWrite > 0 sẽ đánh dấu ngay CacheCapable=true, got %+v", per)
	}
	if !tk.OverallCacheCapable() {
		t.Errorf("tổng thể CacheCapable cũng phải được đặt thành true đồng thời")
	}

	// Đảo ngược: vai trò không có hoạt động bộ đệm nào cả, CacheCapable phải giữ nguyên sai
	tk.Record("editor", makeUsageMsg(500, 0, 0, 100))
	per = tk.PerAgent()
	for _, a := range per {
		if a.Role == "editor" && a.CacheCapable {
			t.Errorf("Trình chỉnh sửa không có CacheRead/Write xuyên suốt và CacheCapable không được đánh dấu sai là true.")
		}
	}
}

// Test_UsageTracker_AccumulatesAnyRoleWithUsage xác minh rằng đường dẫn tích lũy được tách rời khỏi Vai trò:
// Ngay cả khi một bộ chuyển đổi trong tương lai tập hợp việc sử dụng vào một tin nhắn với vai trò không phải là trợ lý,
// Nó vẫn tích lũy chính xác. Giữ khế ước “tách quy tắc hội và quy tắc tích lũy”.
func Test_UsageTracker_AccumulatesAnyRoleWithUsage(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	// Xây dựng một thông báo không hỗ trợ ít phổ biến hơn về mặt lý thuyết với Cách sử dụng
	hypothetical := agentcore.Message{
		Role:  agentcore.RoleSystem,
		Usage: &agentcore.Usage{Input: 200, Output: 50, CacheRead: 100},
	}
	tk.Record("writer", hypothetical)

	_, in, out, cr, _ := tk.Totals()
	if in != 200 || out != 50 || cr != 100 {
		t.Errorf("Trường Cách sử dụng không được tích lũy, đã nhận (in=%d out=%d cr=%d) muốn (200 50 100)", in, out, cr)
	}
	if tk.MissingAssistantUsage() != 0 {
		t.Errorf("Có Cách sử dụng không được tính là thiếu")
	}
}

// Test_UsageTracker_OnCostCallback Xác minh điểm kết nối của trọng điểm ngân sách: sau mỗi lần hạch toán
// Cuộc gọi lại ngoài khóa mang theo chi phí tích lũy mới nhất (bao gồm cả đường dẫn chi phí tự báo cáo của nhà cung cấp).
func Test_UsageTracker_OnCostCallback(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	var got []float64
	tk.SetOnCost(func(total float64) { got = append(got, total) })

	msg := func(cost float64) agentcore.AgentMessage {
		return agentcore.Message{
			Role:  agentcore.RoleAssistant,
			Usage: &agentcore.Usage{Input: 100, Output: 10, Cost: &agentcore.Cost{Total: cost}},
		}
	}
	tk.Record("writer", msg(0.5))
	tk.Record("writer", msg(0.25))

	if len(got) != 2 || got[0] != 0.5 || got[1] != 0.75 {
		t.Fatalf("onCost should carry growing totals, got %v", got)
	}
}

// Test_UsageTracker_OnMissingUsageOnce xác minh rằng lệnh gọi lại vùng chết chỉ được kích hoạt lần đầu tiên.
func Test_UsageTracker_OnMissingUsageOnce(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	fired := 0
	tk.SetOnMissingUsage(func() { fired++ })

	noUsage := agentcore.Message{Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.TextBlock("chữ")}}
	tk.Record("writer", noUsage)
	tk.Record("writer", noUsage)
	tk.Record("editor", noUsage)

	if fired != 1 {
		t.Fatalf("onMissingUsage should fire exactly once, got %d", fired)
	}
}
