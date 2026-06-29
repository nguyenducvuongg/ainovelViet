// Quy tắc gói triển khai lớp đầu vào liên tục (Chính sách) của tùy chọn người dùng.
//
// Quy tắc là loại sự kiện thứ tư, cùng với Progress/Checkpoint/Artifact nhưng có tính chất trái ngược:
// Ba loại đầu tiên là đầu ra của hệ thống và Quy tắc là đầu vào liên tục theo ý định của người dùng.
//
// Hạn chế về thiết kế (không thể thương lượng):
//   - Công cụ chỉ trả về sự kiện chứ không trả về hướng dẫn (Vi phạm là sự thật và người chỉnh sửa quyết định có kích hoạt viết lại hay không)
//   - Không đưa ra đường dẫn phán quyết mới (sử dụng lại PendingRewrites)
//   - Trường mức độ nghiêm trọng không được đưa vào (mức độ nghiêm trọng được ánh xạ cố định theo loại quy tắc và người chỉnh sửa quyết định độc lập về ngữ nghĩa)
//   - Không âm thầm nuốt xung đột (tất cả các trường hợp ngoại lệ được nhập vào Bundle.Conflicts, hiển thị LLM và /diag)
//   - Flow Router không di chuyển (luật không tham gia định tuyến)
package rules

// SourceKind đánh dấu nguồn của các quy tắc và được sử dụng để sắp xếp mức độ ưu tiên gần nhất khi hợp nhất.
// Giá trị càng lớn thì càng gần: Project > Global > Default.
//
// Bắt đầu từ Giai đoạn 1.1, chỉ có ba lớp được hỗ trợ. Lớp Thể loại/Đã học không mở lỗ hổng trước khi thư viện chủ đề/save_rule thực tế được triển khai——
// Nếu bạn thực sự muốn mở rộng, chỉ cần thêm hằng số và bộ tải, không để trống giá trị nào.
type SourceKind int

const (
	// SourceDefault — Các quy tắc mặc định tích hợp sẵn của dự án (asset/rules/default.md), với mức độ ưu tiên thấp nhất.
	SourceDefault SourceKind = iota
	// SourceGlobal — Tùy chọn chung của người dùng (tất cả .mds trong thư mục ~/.ainovel/rules/, được hợp nhất theo thứ tự từ điển của tên tệp), được sử dụng lại trên nhiều cuốn sách.
	SourceGlobal
	// SourceProject — các quy tắc của cuốn sách này (tất cả .mds trong thư mục ..ainovel/rules/, được hợp nhất theo thứ tự từ điển của tên tệp), với mức độ ưu tiên cao nhất.
	SourceProject
)

// Chuỗi Trả về tên nguồn mà con người có thể đọc được, được sử dụng để ghép nối đánh dấu tiêu đề nguồn và các xung đột.chi tiết.
func (k SourceKind) String() string {
	switch k {
	case SourceDefault:
		return "default"
	case SourceGlobal:
		return "global"
	case SourceProject:
		return "project"
	default:
		return "unknown"
	}
}

// WordRange đại diện cho phạm vi số từ của chương được phép; nil đại diện cho việc không được khai báo.
type WordRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Cấu trúc tải trường có cấu trúc của vật chất phía trước.
//
// Khi phân tích cú pháp một tệp duy nhất, Parsed.Structured chỉ điền vào các trường được khai báo trong tệp và phần còn lại giữ nguyên giá trị 0.
// Sau khi hợp nhất, Bundle.Structured là kết quả tổng thể của từng nguồn với mức độ ưu tiên dành cho nguồn gần nhất.
type Structured struct {
	Genre            string         `json:"genre,omitempty"`
	ChapterWords     *WordRange     `json:"chapter_words,omitempty"`
	ForbiddenChars   []string       `json:"forbidden_chars,omitempty"`
	ForbiddenPhrases []string       `json:"forbidden_phrases,omitempty"`
	FatigueWords     map[string]int `json:"fatigue_words,omitempty"`
}

// IsEmpty được sử dụng để xác định xem liệu không có quy tắc có cấu trúc nào cả; việc kiểm tra có thể được bỏ qua tương ứng.
func (s Structured) IsEmpty() bool {
	return s.Genre == "" &&
		s.ChapterWords == nil &&
		len(s.ForbiddenChars) == 0 &&
		len(s.ForbiddenPhrases) == 0 &&
		len(s.FatigueWords) == 0
}

// Xung đột đánh dấu các loại xung đột hoặc ngoại lệ để tạo điều kiện thuận lợi cho việc xử lý phân loại bằng LLM và bảng chẩn đoán.
type ConflictKind string

const (
	// Xung độtParseError - Việc phân tích cú pháp tổng thể của nội dung phía trước không thành công; cơ thể vẫn được tiêm theo sở thích.
	ConflictParseError ConflictKind = "parse_error"
	// Xung độtUnknownField - Người dùng đã viết một trường không được Giai đoạn 1 hỗ trợ (tương thích về phía trước).
	ConflictUnknownField ConflictKind = "unknown_field"
	// Xung độtTypeError - Loại trường sai (ví dụ: Cấm_chars được viết dưới dạng chuỗi); trường này bị loại bỏ.
	ConflictTypeError ConflictKind = "type_error"
	// Xung độtFieldXung đột - Các giá trị trường có cấu trúc giống nhau từ nhiều nguồn không nhất quán; cái gần nhất có hiệu lực trước.
	ConflictFieldConflict ConflictKind = "field_conflict"
	// Xung độtInvalidValue — Định dạng giá trị trường là không hợp lệ (chẳng hạn như chap_words: "abc"); trường này bị loại bỏ.
	ConflictInvalidValue ConflictKind = "invalid_value"
)

// Xung đột Một bản ghi xung đột hoặc ngoại lệ.
//
// Không bao giờ chặn tải - tất cả các trường hợp ngoại lệ đều được hiển thị ở đây với LLM và /diag và không được xử lý một cách im lặng.
type Conflict struct {
	Source string       `json:"source"`          // Đường dẫn tệp (tuyệt đối hoặc tương đối, được ghi theo nguồn)
	Kind   ConflictKind `json:"kind"`            // Loại xung đột
	Field  string       `json:"field,omitempty"` // Tên trường bị ảnh hưởng (chẳng hạn như các ký tự bị cấm); trống cho phân tích cú pháp_error
	Detail string       `json:"detail"`          // Chi tiết mà con người có thể đọc được (có danh sách nguồn/thông báo lỗi)
}

// Đã phân tích cú pháp là kết quả của việc phân tích một bản sao của Rules.md.
type Parsed struct {
	Source     string     // đường dẫn tập tin
	Kind       SourceKind // Loại nguồn để ưu tiên hợp nhất
	Structured Structured // Trường vấn đề phía trước được khai báo trong tệp này
	Preference string     // Phần thân Markdown của tệp (phần bên ngoài phần trước)
	Conflicts  []Conflict // xung đột (lỗi trường/loại không xác định) trong quá trình phân tích cú pháp tệp này
}

// Gói là dạng cuối cùng được đưa vào Working_memory.user_rules sau khi hợp nhất.
//
// Các trường được ánh xạ tới đầu ra JSON:
//
//	{
//	  "structured": {...},
//	  "preferences": "...hợp nhất đánh dấu...",
//	  "sources": ["..."],
//	  "conflicts": [...]
//	}
type Bundle struct {
	Structured  Structured `json:"structured"`
	Preferences string     `json:"preferences"`
	Sources     []string   `json:"sources"`
	Conflicts   []Conflict `json:"conflicts"`
}

// IsEmpty có nghĩa là Gói không có nội dung nào cả (các trường có cấu trúc trống + nội dung tùy chọn trống).
// Vẫn phải để lại một Gói trống khi chèn user_rules để tránh LLM xử lý con số không.
func (b Bundle) IsEmpty() bool {
	return b.Structured.IsEmpty() && b.Preferences == ""
}

// Mức độ nghiêm trọng Đánh dấu mức độ vi phạm nghiêm trọng.
// Đã sửa lỗi ánh xạ (người dùng không thể định cấu hình):
//
//	cấm_chars xuất hiện -> Lỗi
//	cấm_cụm từ xuất hiện -> Lỗi
//	mệt_words vượt quá ngưỡng -> Cảnh báo
//	độ lệch chap_words < 20% -> Cảnh báo
//	độ lệch chap_words >= 20% -> Lỗi
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// ChapterWordsDeviationThreshold xác định giá trị tới hạn (20%) mà tại đó độ lệch của chap_words được nâng cấp thành lỗi.
const ChapterWordsDeviationThreshold = 0.20

// Vi phạm là đầu ra của trình kiểm tra: một tuyên bố thực tế rằng chương này vi phạm quy tắc cơ học.
//
// Lưu ý: commit_chapter truyền các vi phạm tới JSON được trả về một cách minh bạch và không chặn cam kết;
// Người biên tập ánh xạ những dữ kiện này vào bảy khía cạnh hiện có (thẩm mỹ/nhịp độ/đặc điểm/tính nhất quán) trong quá trình xem xét,
// LLM có quyền quyết định có nên nâng cấp phán quyết để kích hoạt đánh bóng/viết lại hay không.
type Violation struct {
	Rule      string   `json:"rule"`                // forbidden_chars / forbidden_phrases / fatigue_words / chapter_words
	Target    string   `json:"target,omitempty"`    // Đối tượng vi phạm cụ thể (từ/ký tự nào); chap_words được để trống
	Limit     any      `json:"limit,omitempty"`     // Ngưỡng; mệt mỏi_words=int / chap_words="3000-6000" / cấm_*=null
	Actual    any      `json:"actual"`              // Giá trị thực tế; mệt mỏi_words/forbidden_*=số lần xuất hiện/chương_words=số từ trong chương này
	Deviation float64  `json:"deviation,omitempty"` // tỷ lệ sai lệch chap_words (0~1), để trống các quy tắc khác
	Severity  Severity `json:"severity"`            // error / warning
}
