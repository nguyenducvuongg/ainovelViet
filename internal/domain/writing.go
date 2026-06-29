package domain

// Ý tưởng viết chương ChapterPlan, được tạo ra một cách độc lập bởi Writer.
// Không còn tình trạng bắt buộc phải chia cảnh, Agent quyết định cách sắp xếp nội dung.
type ChapterPlan struct {
	Chapter    int             `json:"chapter"`
	Title      string          `json:"title"`
	Goal       string          `json:"goal"`
	Conflict   string          `json:"conflict"`
	Hook       string          `json:"hook"`
	EmotionArc string          `json:"emotion_arc,omitempty"`
	Notes      string          `json:"notes,omitempty"` // Bản ghi nhớ miễn phí của đại lý
	Contract   ChapterContract `json:"contract,omitempty"`
}

// Hợp đồng Chương là hợp đồng chấp nhận chương được chia sẻ bởi Người viết và Người biên tập.
// Nó xác định những gì chương phải hoàn thành, những gì không nên vượt qua và những gì cần tập trung vào để ôn tập.
type ChapterContract struct {
	RequiredBeats    []string `json:"required_beats,omitempty"`    // Các mục thăng tiến phải được thực hiện trong chương này
	ForbiddenMoves   []string `json:"forbidden_moves,omitempty"`   // Chương này nói rõ rằng sự thăng tiến không thể xảy ra
	ContinuityChecks []string `json:"continuity_checks,omitempty"` // Các điểm liên tục cần kiểm tra đặc biệt trong chương này
	EvaluationFocus  []string `json:"evaluation_focus,omitempty"`  // Những điểm soạn thảo cần được kiểm tra
	EmotionTarget    string   `json:"emotion_target,omitempty"`    // Tùy chọn: Những cảm xúc chính mà bạn muốn người đọc cảm nhận được trong chương này
	PayoffPoints     []string `json:"payoff_points,omitempty"`     // Tùy chọn: Vẽ điểm/điểm thực hiện mà bạn muốn phản hồi trong chương chính
	HookGoal         string   `json:"hook_goal,omitempty"`         // Tùy chọn: câu kết cuối chương thúc đẩy ham muốn đọc
}

// ChươngTóm tắtTóm tắt chương để sử dụng trong cửa sổ ngữ cảnh của các chương tiếp theo.
type ChapterSummary struct {
	Chapter    int      `json:"chapter"`
	Summary    string   `json:"summary"`
	Characters []string `json:"characters"`
	KeyEvents  []string `json:"key_events"`
}

// ArcSummary Tóm tắt cấp độ Arc, được Editor tạo ra ở cuối cung.
type ArcSummary struct {
	Volume    int      `json:"volume"`
	Arc       int      `json:"arc"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	KeyEvents []string `json:"key_events"`
}

// VolumeSummary Tóm tắt cấp độ tập, được tạo ở cuối tập.
type VolumeSummary struct {
	Volume    int      `json:"volume"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	KeyEvents []string `json:"key_events"`
}

// Ảnh chụp nhanh trạng thái ký tự, được ghi ở ranh giới cung.
type CharacterSnapshot struct {
	Volume     int    `json:"volume"`
	Arc        int    `json:"arc"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Power      string `json:"power,omitempty"`
	Motivation string `json:"motivation"`
	Relations  string `json:"relations,omitempty"`
}

// Đề cươngPhản hồiPhản hồi của người viết về dàn ý, tùy chọn khi gửi một chương.
type OutlineFeedback struct {
	Deviation  string `json:"deviation"`  // sai lệch so với mô tả
	Suggestion string `json:"suggestion"` // Đề xuất điều chỉnh
}

// WritingStyleRules Quy tắc viết được trích xuất từ ​​các chương đã viết, được Trình soạn thảo tạo ra tại các ranh giới cung.
// Thay thế các đoạn văn bản gốc (style_anchors/voice_samples) và sử dụng các quy tắc để thay thế văn bản gốc.
type WritingStyleRules struct {
	Volume    int              `json:"volume"`
	Arc       int              `json:"arc"`
	Prose     []string         `json:"prose"`      // 3-5 quy tắc về phong cách tường thuật, mỗi quy tắc 50 từ
	Dialogue  []CharacterVoice `json:"dialogue"`   // Quy tắc phong cách đối thoại nhân vật
	Taboos    []string         `json:"taboos"`     // danh sách cấm kỵ
	UpdatedAt string           `json:"updated_at"` // Dấu thời gian ISO8601
}

// Quy tắc kiểu hội thoại CharacterVoice cho một ký tự.
type CharacterVoice struct {
	Name  string   `json:"name"`
	Rules []string `json:"rules"` // 2-3 quy tắc đặc trưng ngôn ngữ, mỗi quy tắc 30 từ
}

// Chương liên quan Đề xuất các chương liên quan để đọc lại.
type RelatedChapter struct {
	Chapter int    `json:"chapter"`
	Reason  string `json:"reason"`
}

// RecallItem là thông tin dài hạn được thu hồi có chọn lọc theo nhiệm vụ hiện tại.
// Nó không thay thế các tạo phẩm chính thức mà chỉ chịu trách nhiệm đưa một lượng nhỏ thông tin lịch sử thực sự có liên quan đến vòng hiện tại vào mô hình.
type RecallItem struct {
	Kind    string `json:"kind"`
	Key     string `json:"key,omitempty"`
	Chapter int    `json:"chapter,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// CommitResult là giá trị trả về có cấu trúc của công cụ commit_chapter.
// Chỉ chứa các trường dữ kiện; "Việc cần làm tiếp theo" do chính kênh Nhắc nhở tạo ra dựa trên Tiến trình hiện tại.
type CommitResult struct {
	Chapter        int              `json:"chapter"`
	Committed      bool             `json:"committed"`
	WordCount      int              `json:"word_count"`
	NextChapter    int              `json:"next_chapter"`
	ReviewRequired bool             `json:"review_required"`
	ReviewReason   string           `json:"review_reason,omitempty"`
	HookType       string           `json:"hook_type,omitempty"`
	DominantStrand string           `json:"dominant_strand,omitempty"`
	Feedback       *OutlineFeedback `json:"feedback,omitempty"`
	// tín hiệu lớp dài
	ArcEnd         bool `json:"arc_end,omitempty"`
	VolumeEnd      bool `json:"volume_end,omitempty"`
	Volume         int  `json:"volume,omitempty"`
	Arc            int  `json:"arc,omitempty"`
	NeedsExpansion bool `json:"needs_expansion,omitempty"`  // Phần tiếp theo là bộ xương và các chương cần được mở rộng.
	NeedsNewVolume bool `json:"needs_new_volume,omitempty"` // Kiến trúc sư được yêu cầu để tạo tập tiếp theo
	NextVolume     int  `json:"next_volume,omitempty"`      // Số cung/tập tiếp theo
	NextArc        int  `json:"next_arc,omitempty"`         // Số cung tiếp theo
	// Thông tin về trạng thái hoàn thành: Liệu toàn bộ cuốn sách có được hoàn thành sau lần cam kết này hay không
	BookComplete bool `json:"book_complete,omitempty"`
	// Ảnh chụp nhanh tiến độ hiện tại.Flow (viết / xem lại / viết lại / đánh bóng)
	Flow string `json:"flow,omitempty"`
}
