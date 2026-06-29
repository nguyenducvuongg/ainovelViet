package domain

import "strings"

// Giai đoạn đại diện cho giai đoạn sáng tạo tiểu thuyết.
type Phase string

const (
	PhaseInit     Phase = "init"
	PhasePremise  Phase = "premise"
	PhaseOutline  Phase = "outline"
	PhaseWriting  Phase = "writing"
	PhaseComplete Phase = "complete"
)

// FlowState Loại quy trình đang hoạt động hiện tại, được sử dụng để khôi phục điểm kiểm tra.
type FlowState string

const (
	FlowWriting   FlowState = "writing"
	FlowReviewing FlowState = "reviewing"
	FlowRewriting FlowState = "rewriting"
	FlowPolishing FlowState = "polishing"
	FlowSteering  FlowState = "steering"
)

// PlanningTier thể hiện mức độ dài của kế hoạch công việc.
type PlanningTier string

const (
	PlanningTierShort PlanningTier = "short"
	PlanningTierMid   PlanningTier = "mid"
	PlanningTierLong  PlanningTier = "long"
)

// Theo dõi tiến độ tiến độ, được duy trì ở meta/progress.json.
type Progress struct {
	NovelName         string      `json:"novel_name"`
	Phase             Phase       `json:"phase"`
	CurrentChapter    int         `json:"current_chapter"`
	TotalChapters     int         `json:"total_chapters"`
	CompletedChapters []int       `json:"completed_chapters"`
	TotalWordCount    int         `json:"total_word_count"`
	ChapterWordCounts map[int]int `json:"chapter_word_counts,omitempty"` // Số từ mỗi chương, hỗ trợ chỉnh sửa tổng số từ khi viết lại
	InProgressChapter int         `json:"in_progress_chapter,omitempty"` // Chương đang được tiến hành (khôi phục ở cấp độ cảnh)
	CompletedScenes   []int       `json:"completed_scenes,omitempty"`    // Số cảnh đã hoàn thành của chương hiện tại
	Flow              FlowState   `json:"flow,omitempty"`                // quá trình hiện tại
	PendingRewrites   []int       `json:"pending_rewrites,omitempty"`    // Hàng đợi các chương được viết lại
	RewriteReason     string      `json:"rewrite_reason,omitempty"`      // Lý do viết lại
	StrandHistory     []string    `json:"strand_history,omitempty"`      // Ghi lại trội_strand theo thứ tự chương
	HookHistory       []string    `json:"hook_history,omitempty"`        // Ghi hook_type theo thứ tự chương
	// Theo dõi phân cấp truyện dài (chỉ được sử dụng trong chế độ truyện dài, truyện ngắn/trung bình có giá trị bằng 0)
	CurrentVolume int  `json:"current_volume,omitempty"`
	CurrentArc    int  `json:"current_arc,omitempty"`
	Layered       bool `json:"layered,omitempty"`
	// ReopenedFromComplete đánh dấu cuốn sách đã được mở lại từ trạng thái hoàn thành và được đưa vào làm lại. Làm lại chỉ thay đổi các chương hiện có,
	// Không có sự bổ sung hay xóa bỏ cấu trúc nên sau khi làm trống nên phát hành theo nguyên tắc “hoàn thiện lại nếu cấu trúc hoàn chỉnh” (để tránh điềm báo cuối tập cuối bị xáo trộn do làm lại và mắc kẹt.
	// viết → tiếp tục vượt quá giới hạn của một vòng lặp vô hạn); việc viết tiếp không đặt ra dấu ấn này và phán quyết hoàn thành vẫn duy trì ngữ nghĩa thận trọng của việc đóng đầu mối.
	ReopenedFromComplete bool `json:"reopened_from_complete,omitempty"`
}

// IsResumable xác định liệu nó có thể được tiếp tục lại từ điểm dừng hay không.
func (p *Progress) IsResumable() bool {
	return p.Phase == PhaseWriting && p.CurrentChapter > 0
}

// NextChapter trả về số chương tiếp theo sẽ được viết.
func (p *Progress) NextChapter() int {
	return p.LatestCompleted() + 1
}

// LastCompleted trả về số chương đã hoàn thành tối đa; trả về 0 nếu không có chương nào được hoàn thành.
func (p *Progress) LatestCompleted() int {
	max := 0
	for _, ch := range p.CompletedChapters {
		if ch > max {
			max = ch
		}
	}
	return max
}

// ExtractNovelNameFromPremise trích xuất tên sách từ dòng đầu tiên của tiền đề `# tên sách` (có thể được gói bằng "").
// Mô hình đôi khi sao chép phần giữ chỗ trong các từ nhắc thay vì tạo ra tên thật. Các giá trị này được xử lý như thể chúng chưa được trích xuất và trả về giá trị trống.
// Để ở lớp trên (giao diện hiển thị “Undecided Book Title”) để tránh hiển thị trực tiếp chữ “Book Title” trên giao diện.
func ExtractNovelNameFromPremise(premise string) string {
	for raw := range strings.SplitSeq(strings.ReplaceAll(premise, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			return ""
		}
		name := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "# ")), "《》\"")
		nameLower := strings.ToLower(name)
		switch nameLower {
		case "tên sách", "tên sách thực tế", "tiêu đề sách ví dụ", "tiêu đề sách mẫu":
			return "" // Giữ chỗ từ nhắc nhở, không phải tên sách thật
		}
		return name
	}
	return ""
}

// Chiến lược tải ngữ cảnh ContextProfile, thích ứng dựa trên tổng số chương.
type ContextProfile struct {
	SummaryWindow  int  // Tải tóm tắt N chương cuối cùng
	TimelineWindow int  // Tải dòng thời gian chương N mới nhất
	Layered        bool // true = Cho phép tải tóm tắt phân cấp (tóm tắt tập + tóm tắt cung + tóm tắt chương)
}

// MemoryPolicy thể hiện chính sách sử dụng bộ nhớ dùng chung trong thời gian chạy.
// Nó được sử dụng cho cả đầu ra ngữ cảnh và cho các quyết định chuyển giao/nhắc nhở của lớp máy chủ.
type MemoryPolicy struct {
	Mode                string `json:"mode,omitempty"`
	SummaryWindow       int    `json:"summary_window,omitempty"`
	TimelineWindow      int    `json:"timeline_window,omitempty"`
	LayeredSummaries    bool   `json:"layered_summaries,omitempty"`
	SummaryStrategy     string `json:"summary_strategy,omitempty"`
	WorkingRefresh      string `json:"working_refresh,omitempty"`
	EpisodicRefresh     string `json:"episodic_refresh,omitempty"`
	PlanningRefresh     string `json:"planning_refresh,omitempty"`
	FoundationRefresh   string `json:"foundation_refresh,omitempty"`
	PlanningFocus       string `json:"planning_focus,omitempty"`
	FoundationFocus     string `json:"foundation_focus,omitempty"`
	PreviousTailChars   int    `json:"previous_tail_chars,omitempty"`
	ChapterPlanEnabled  bool   `json:"chapter_plan_enabled,omitempty"`
	RelatedLookup       bool   `json:"related_chapter_lookup,omitempty"`
	CurrentOutlineBound bool   `json:"current_outline_bound,omitempty"`
	TotalChapters       int    `json:"total_chapters,omitempty"`
	HandoffPreferred    bool   `json:"handoff_preferred,omitempty"`
	ReadOnlyThreshold   int    `json:"read_only_threshold,omitempty"`
}

// NewContextProfile Tính toán chính sách ngữ cảnh dựa trên tổng số chương.
func NewContextProfile(totalChapters int) ContextProfile {
	switch {
	case totalChapters <= 15:
		return ContextProfile{SummaryWindow: 10, TimelineWindow: 10}
	case totalChapters <= 50:
		return ContextProfile{SummaryWindow: 5, TimelineWindow: 8}
	default:
		return ContextProfile{SummaryWindow: 3, TimelineWindow: 5, Layered: true}
	}
}

// NewChapterMemoryPolicy tạo ra các chính sách bộ nhớ thời gian chạy chương dựa trên các chính sách về tiến trình và ngữ cảnh.
func NewChapterMemoryPolicy(progress *Progress, profile ContextProfile, currentOutlineBound bool) MemoryPolicy {
	policy := MemoryPolicy{
		Mode:                "chapter",
		SummaryWindow:       profile.SummaryWindow,
		TimelineWindow:      profile.TimelineWindow,
		LayeredSummaries:    profile.Layered,
		WorkingRefresh:      "Làm mới mỗi khi chương được tải",
		EpisodicRefresh:     "Được làm mới với các lần gửi chương, đánh giá và thay đổi trạng thái tính năng",
		PreviousTailChars:   800,
		ChapterPlanEnabled:  true,
		CurrentOutlineBound: currentOutlineBound,
		ReadOnlyThreshold:   5,
	}
	if profile.Layered {
		policy.SummaryStrategy = "Tóm tắt tập + Tóm tắt Arc + Tóm tắt chương gần đây"
	} else {
		policy.SummaryStrategy = "Tóm tắt các chương gần đây"
	}
	if progress != nil {
		policy.TotalChapters = progress.TotalChapters
		if progress.TotalChapters > 30 {
			policy.RelatedLookup = true
		}
		if progress.Flow == FlowReviewing || progress.Flow == FlowRewriting || progress.Flow == FlowPolishing {
			policy.HandoffPreferred = true
		}
		if progress.Layered && len(progress.CompletedChapters) >= 6 {
			policy.HandoffPreferred = true
		}
		if len(progress.CompletedChapters) >= 12 {
			policy.HandoffPreferred = true
		}
		if progress.Layered && len(progress.CompletedChapters) >= 6 {
			policy.ReadOnlyThreshold = 4
		}
		if len(progress.CompletedChapters) >= 12 {
			policy.ReadOnlyThreshold = 4
		}
	}
	return policy
}

// NewArchitectMemoryPolicy Trả về chính sách bộ nhớ được sử dụng trong giai đoạn lập kế hoạch.
func NewArchitectMemoryPolicy() MemoryPolicy {
	return MemoryPolicy{
		Mode:               "architect",
		PlanningRefresh:    "Làm mới khi cấu trúc vòng cung, la bàn hoặc tóm tắt được cập nhật",
		FoundationRefresh:  "Làm mới khi ký tự, điềm báo và cài đặt thay đổi",
		PlanningFocus:      "Phác thảo lớp, la bàn, tóm tắt khối lượng",
		FoundationFocus:    "Cài đặt nhân vật, ảnh chụp nhân vật, tài khoản báo trước",
		HandoffPreferred:   true,
		ChapterPlanEnabled: false,
		ReadOnlyThreshold:  4,
	}
}

// RunMeta chạy thông tin meta và được lưu giữ ở meta/run.json.
type RunMeta struct {
	StartedAt    string       `json:"started_at"`
	Provider     string       `json:"provider,omitempty"`
	Style        string       `json:"style"`
	Model        string       `json:"model"`
	PlanningTier PlanningTier `json:"planning_tier,omitempty"`
	SteerHistory []SteerEntry `json:"steer_history,omitempty"`
	PendingSteer string       `json:"pending_steer,omitempty"` // Hướng dẫn chỉ đạo chưa hoàn thành, được tiêm lại khi phục hồi bị gián đoạn
}

// Ghi nhật ký can thiệp của người dùng SteerEntry.
type SteerEntry struct {
	Input     string `json:"input"`
	Timestamp string `json:"timestamp"`
}

// Các yêu cầu tạo dài hạn do người dùng UserDirective đưa ra sẽ tiếp tục có hiệu lực trong các chương.
// Vẫn tồn tại trong meta/user_directives.json, được đưa vào bởi Novel_context
// Working_memory.user_directives để tất cả các tác nhân phụ tuân thủ.
//
// Chương/TotalChapters là ảnh chụp nhanh về tiến trình khi nó được ban hành: hãy để hướng dẫn có điểm bắt đầu rõ ràng để có hiệu lực (không có hiệu lực hồi tố).
// các chương trước), nó cũng cho phép người đọc đánh giá là người đọc hài lòng với các hướng dẫn tương đối bị lưu sai (chẳng hạn như "Thêm 10 chương").
// Thay vì thực hiện lại nó mỗi khi nó được đọc lại.
type UserDirective struct {
	Text          string `json:"text"`
	Chapter       int    `json:"chapter"`        // Tiến độ viết tại thời điểm phát hành
	TotalChapters int    `json:"total_chapters"` // Tổng số chương quy hoạch tại thời điểm phát hành
	CreatedAt     string `json:"created_at"`     // RFC3339
}
