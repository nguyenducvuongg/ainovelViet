package diag

// Mức độ nghiêm trọng cho thấy mức độ nghiêm trọng của phát hiện.
type Severity string

const (
	SevCritical Severity = "critical" // Tiến trình bị chặn hoặc hỏng dữ liệu
	SevWarning  Severity = "warning"  // Có thể làm giảm chất lượng hoặc lãng phí token
	SevInfo     Severity = "info"     // Các mục có thể tối ưu hóa
)

// Danh mục nhóm các phát hiện theo thứ nguyên.
type Category string

const (
	CatFlow     Category = "flow"     // Độ trễ của quá trình, trạng thái bất thường và sự cố khôi phục
	CatQuality  Category = "quality"  // Điểm đánh giá, hiệu suất hợp đồng, tính nhất quán
	CatPlanning Category = "planning" // Phác thảo những khoảng trống, sự trôi dạt báo trước và sự lỗi thời của la bàn
	CatContext  Category = "context"  // Những bất thường về nhân vật/dòng thời gian/mối quan hệ
)

// Độ tin cậy thể hiện mức độ tin cậy của việc xác định quy tắc.
type Confidence string

const (
	ConfHigh   Confidence = "high"   // Sự chắc chắn và đáng tin cậy mạnh mẽ
	ConfMedium Confidence = "medium" // Phán đoán theo kinh nghiệm, có thể có những phán đoán sai lầm
	ConfLow    Confidence = "low"    // Tín hiệu thô, chỉ mang tính chất tham khảo
)

// AutoLevel cho biết liệu việc Tìm kiếm có thể được chuyển đổi thành hành động tự động hay không.
type AutoLevel string

const (
	AutoNone    AutoLevel = "none"    // Chỉ báo cáo, không tự động
	AutoSuggest AutoLevel = "suggest" // Hành động được đề xuất nhưng yêu cầu xác nhận thủ công
	AutoSafe    AutoLevel = "safe"    // An toàn và tự động
)

// Tìm kiếm là kết quả chẩn đoán có thể thực thi được.
type Finding struct {
	Rule       string     // Tên quy tắc, chẳng hạn như "StaleForeshadow"
	Category   Category   // Phân loại
	Severity   Severity   // Mức độ nghiêm trọng
	Confidence Confidence // Sự tự tin quyết định
	AutoLevel  AutoLevel  // Mức độ tự động hóa
	Target     string     // Phạm vi được đề xuất, chẳng hạn như "runtime.flow"
	Title      string     // tóm tắt một dòng
	Evidence   string     // bằng chứng dữ liệu cụ thể
	Suggestion string     // Đề xuất cải tiến (trỏ tới dấu nhắc/luồng/cấu hình)
}

// RuleFunc là chữ ký thống nhất cho các quy tắc chẩn đoán.
type RuleFunc func(snap *Snapshot) []Finding

// ActionKind đại diện cho loại hành động chẩn đoán.
type ActionKind string

const (
	ActionEmitNotice      ActionKind = "emit_notice"       // Gửi lời nhắc hệ thống
	ActionEnqueueFollowUp ActionKind = "enqueue_follow_up" // Tiêm theo dõi điều phối viên
)

// Hành động là các hành động thực thi được Planner tạo ra dựa trên các Kết quả có độ tin cậy cao.
type Action struct {
	SourceRule  string     // Tên quy tắc nguồn
	Kind        ActionKind // kiểu hành động
	Severity    Severity   // Kế thừa từ việc tìm kiếm
	Summary     string     // mô tả ngắn
	Message     string     // Tin nhắn được chuyển đến luồng điều khiển
	Fingerprint string     // Dấu vân tay ổn định của Tìm nguồn để chống trùng lặp thời gian chạy
}

// Số liệu thống kê là số liệu tổng quan được hiển thị cùng với các phát hiện.
type Stats struct {
	CompletedChapters int
	TotalChapters     int
	TotalWords        int
	AvgWordsPerCh     int
	Phase             string
	Flow              string
	PlanningTier      string
	ReviewCount       int
	RewriteCount      int
	AvgReviewScore    float64
	ForeshadowOpen    int
	ForeshadowStale   int
}

// Báo cáo là kết quả đầu ra hoàn chỉnh của một lần chạy chẩn đoán.
type Report struct {
	Stats    Stats
	Findings []Finding
	Actions  []Action
}
