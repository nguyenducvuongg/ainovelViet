package host

import (
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// Sự kiện là các sự kiện có cấu trúc được TUI sử dụng.
//
// Đối với TOOL / DISPATCH hai loại sự kiện cuộc gọi, sự bắt đầu và kết thúc của cùng một cuộc gọi có cùng một ID:
// Lúc đầu, sự kiện có giá trị FinishedAt là 0 sẽ được gửi trước tiên (TUI được hiển thị dưới dạng kiểu "đang xử lý");
// Cuối cùng, hãy gửi một sự kiện khác có cùng ID và điền vào FinishedAt + Duration (+ Failed).
// TUI định vị hàng gốc theo ID và cập nhật nó tại chỗ để tránh tình trạng dư thừa "bắt đầu một hàng và hoàn thành một hàng khác".
//
// ID của các sự kiện không gọi điện như HỆ THỐNG / LỖI / TIẾP THEO trống và mỗi sự kiện được thêm độc lập.
type Event struct {
	ID         string    // Việc bắt đầu/kết thúc của cùng một cuộc gọi được chia sẻ; sự kiện không có cuộc gọi trống
	Time       time.Time // Lần phát hành đầu tiên (thời gian bắt đầu)
	FinishedAt time.Time // Giá trị 0 = đang tiến hành; khác 0 = đã hoàn thành
	Failed     bool      // Đã hoàn thành nhưng không thành công (chỉ trạng thái hoàn thành mới có ý nghĩa)
	Category   string    // DISPATCH / TOOL / SYSTEM / REVIEW / CHECK / ERROR / CONTEXT
	Agent      string    // Tác nhân tạo ra sự kiện
	Summary    string
	Detail     string        // Bản sao hoàn chỉnh, được ghi vào nhật ký mà không cắt bớt để khắc phục sự cố; quay lại Tóm tắt nếu trống. Giao diện người dùng chỉ đọc
	Kind       string        // Phân loại lỗi (chẳng hạn như stream_idle), xuất ra nhật ký để lọc/cảnh báo; nếu trống, không có đầu ra
	Level      string        // info / warn / error / success
	Depth      int           // 0 = lớp điều phối viên, 1 = lớp tác nhân phụ
	Duration   time.Duration // Thời gian thực hiện đến khi hoàn thành
}

// Đang chạy Trả về xem sự kiện có đang diễn ra hay không.
// Chỉ có thể tiến hành gọi các sự kiện của lớp (TOOL / DISPATCH with ID); các loại khác luôn trả về sai.
func (e Event) Running() bool {
	return e.ID != "" && e.FinishedAt.IsZero()
}

// UISnapshot là ảnh chụp nhanh trạng thái tổng hợp cần thiết để hiển thị TUI.
type UISnapshot struct {
	Provider           string
	NovelName          string
	ModelName          string
	ModelContextWindow int // Cửa sổ ngữ cảnh của mô hình mặc định hiện tại (phân tích cú pháp thời gian thực bằng chuyển đổi /model)
	Style              string
	RuntimeState       string // idle / running / pausing / paused / completed
	StatusLabel        string
	Phase              string
	Flow               string
	CurrentChapter     int
	TotalChapters      int
	CompletedCount     int
	TotalWordCount     int
	InProgressChapter  int
	PendingRewrites    []int
	RewriteReason      string
	PendingSteer       string
	RecoveryLabel      string
	IsRunning          bool
	Agents             []AgentSnapshot

	// bối cảnh
	ContextTokens         int
	ContextWindow         int
	ContextPercent        float64
	ContextScope          string
	ContextStrategy       string
	ContextActiveMessages int
	ContextSummaryCount   int
	ContextCompactedCount int
	ContextKeptCount      int

	// Mức sử dụng tích lũy (toàn bộ phiên, trên tất cả các tổng đài viên và bộ chuyển đổi mô hình)
	TotalInputTokens      int
	TotalOutputTokens     int
	TotalCacheReadTokens  int
	TotalCacheWriteTokens int
	TotalCostUSD          float64
	TotalSavedUSD         float64 // Số đô la được tiết kiệm nhờ lượt truy cập CacheRead (so với giá đầu vào hoàn toàn không được lưu trong bộ nhớ đệm)
	BudgetLimitUSD        float64 // Giới hạn ngân sách (config budget.book_usd); 0 = không được kích hoạt

	// chẩn đoán bộ đệm
	OverallCacheCapable    bool // Ít nhất một vai trò chạy qua mô hình hỗ trợ bộ nhớ đệm nhắc nhở (phân biệt giữa "không bật" và "0% lần truy cập")
	OverallRecentCacheRead int  // Tổng N cacheReads mới nhất trong cửa sổ trượt
	OverallRecentInput     int  // Tổng N đầu vào gần đây nhất của cửa sổ trượt
	OverallRecentSamples   int  // Số lượng mẫu trong cửa sổ trượt ( £ SampleCap gần đây )

	// MissingAssistantUsage > 0 thường có nghĩa là tính năng phát trực tuyến ngược dòng không được OpenAI hỗ trợ
	// Giao thức stream_options.include_usage gửi đoạn sử dụng cuối cùng (phổ biến cho proxy tự xây dựng).
	// Do đó, UsageTracker không thể nhận bất kỳ dữ liệu tích lũy nào. Giao diện người dùng chỉ rõ cho người dùng phần phụ trợ khắc phục sự cố tương ứng.
	// Đừng để người dùng lầm tưởng rằng chính mô-đun bộ đệm đã bị hỏng.
	MissingAssistantUsage int

	// Thứ nguyên bộ đệm cho mỗi vai trò, theo thứ tự giảm dần của CacheRead, vai trò được lọc của mã thông báo chưa được sử dụng
	CachePerAgent []AgentCacheStat
	CachePerModel []AgentCacheStat

	// Cài đặt cơ bản
	Premise          string
	Outline          []OutlineSnapshot
	Characters       []string
	SupportingCount  int      // Tổng số nhân vật phụ trong dàn diễn viên phụ
	RecentSupporting []string // Ký tự phụ hoạt động gần đây nhất (tối đa 5, theo thứ tự giảm dần theo LastSeenChapter)
	Layered          bool
	CurrentVolumeArc string
	NextVolumeTitle  string
	CompassDirection string
	CompassScale     string

	// Chi tiết
	LastCommitSummary  string
	LastReviewSummary  string
	LastCheckpointName string
	RecentSummaries    []string
}

// OutlineSnapshot là bản tóm tắt hiển thị của một mục phác thảo.
type OutlineSnapshot struct {
	Chapter   int
	Title     string
	CoreEvent string
}

// AgentSnapshot là sự thể hiện trạng thái của Tác nhân.
type AgentSnapshot struct {
	Name      string
	State     string
	TaskID    string
	TaskKind  string
	Summary   string
	Tool      string
	Turn      int
	Context   AgentContextSnapshot
	UpdatedAt time.Time
}

// AgentCacheStat là số lần truy cập bộ nhớ đệm tích lũy cho một tác nhân duy nhất (được chiếu vào cột bên trái).
// HitRate = CacheRead / Đầu vào; Đầu vào đã được hợp nhất thành ngữ nghĩa "với CacheRead" ở lớp Litellm.
//
// CacheCapable được sử dụng để phân biệt hai loại lượt truy cập 0%:
//   - true → Model hỗ trợ nhắc nhở cache, 0% nghĩa là thiết kế nhắc nhở kém hoặc tiền tố không ổn định và cần được tối ưu hóa.
//   - false → model/nhà cung cấp không hỗ trợ bộ nhớ đệm nhắc nhở, dự kiến ​​là 0%, không cần kiểm tra
//
// Gần đây* là dữ liệu lần truy cập của cửa sổ trượt (N cuộc gọi cuối cùng). So sánh tích lũy có thể xác định "lực kéo sớm" và "lượt truy cập thấp ở trạng thái ổn định".
type AgentCacheStat struct {
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

// AgentContextSnapshot là cách sử dụng bối cảnh của Tác nhân.
type AgentContextSnapshot struct {
	Tokens          int
	ContextWindow   int
	Percent         float64
	Scope           string
	Strategy        string
	ActiveMessages  int
	SummaryMessages int
	CompactedCount  int
	KeptCount       int
}

// CoCreateMessage là một tin nhắn để cùng tạo một cuộc trò chuyện.
type CoCreateMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CoCreateReply là câu trả lời LLM cho cuộc trò chuyện đồng sáng tạo. Raw giữ lại bốn đoạn văn hoàn chỉnh của văn bản gốc của mô hình,
// Được sử dụng để viết lại lịch sử để những người mẫu tiếp theo có thể xem vòng [DRAFT] trước đó của họ, để thực sự
// Cập nhật tích lũy trên các bản nháp hiện có (chỉ Tin nhắn không chứa [DRAFT], điều này sẽ khiến mô hình được tạo lại dựa trên đoạn hội thoại trong mỗi vòng).
// Gợi ý là “điều bạn có thể muốn nói tiếp theo” do AI chủ động đưa ra. Khi người dùng gặp khó khăn, hãy nhấn các phím số để điền vào ô nhập liệu chỉ bằng một cú nhấp chuột.
type CoCreateReply struct {
	Message     string
	Prompt      string
	Ready       bool
	Suggestions []string
	Raw         string
}

// ReplayDeltaText Trích xuất văn bản phát trực tuyến có thể phát được từ một mục hàng đợi thời gian chạy.
func ReplayDeltaText(item domain.RuntimeQueueItem) string {
	if payload, ok := item.Payload.(map[string]any); ok {
		if text, ok := payload["delta"].(string); ok {
			return text
		}
	}
	return ""
}

// BuildStartPrompt gói các yêu cầu của người dùng vào lời nhắc khởi động của Điều phối viên.
func BuildStartPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	return "Hãy bắt đầu viết một cuốn tiểu thuyết dựa trên các yêu cầu viết sau đây. Sau khi nhập quy hoạch, dòng đầu tiên của Premise phải xuất ra `#tên sách`. Số lượng chương tùy bạn quyết định dựa trên nhu cầu của câu chuyện; nếu chủ đề và xung đột tự nhiên phù hợp với việc xuất bản dài kỳ, vui lòng ưu tiên lập kế hoạch cho cấu trúc dài có thứ bậc thay vì nén nó thành một bản tóm tắt ngắn. \n\n[Yêu cầu về quảng cáo]\n" +
		prompt +
		"\n\nNếu một số chi tiết chưa rõ ràng, vui lòng tự điền mà không vi phạm hướng dẫn của người dùng."
}
