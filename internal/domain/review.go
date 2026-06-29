package domain

// Sự kiện dòng thời gian Sự kiện dòng thời gian.
type TimelineEvent struct {
	Chapter    int      `json:"chapter"`
	Time       string   `json:"time"`
	Event      string   `json:"event"`
	Characters []string `json:"characters,omitempty"`
}

// Mục nhập báo trước Mục nhập báo trước.
type ForeshadowEntry struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	PlantedAt   int    `json:"planted_at"`
	Status      string `json:"status"` // planted / advanced / resolved
	ResolvedAt  int    `json:"resolved_at,omitempty"`
}

// ForeshadowUpdate báo trước hoạt động gia tăng.
type ForeshadowUpdate struct {
	ID          string `json:"id"`
	Action      string `json:"action"` // plant / advance / resolve
	Description string `json:"description,omitempty"`
}

// Mục nhập mối quan hệ Mục nhập mối quan hệ cá nhân.
type RelationshipEntry struct {
	CharacterA string `json:"character_a"`
	CharacterB string `json:"character_b"`
	Relation   string `json:"relation"`
	Chapter    int    `json:"chapter"`
}

// Vấn đề về tính nhất quán Vấn đề về tính nhất quán.
type ConsistencyIssue struct {
	Type        string `json:"type"`     // consistency / character / pacing / continuity / foreshadow / hook / aesthetic
	Severity    string `json:"severity"` // critical / error / warning
	Description string `json:"description"`
	Evidence    string `json:"evidence,omitempty"` // Bằng chứng: các đoạn văn bản gốc, các sơ đồ cụ thể hoặc dữ liệu trạng thái
	Suggestion  string `json:"suggestion,omitempty"`
}

// DimensionScore Điểm đánh giá một chiều.
type DimensionScore struct {
	Dimension string `json:"dimension"`         // consistency / character / pacing / continuity / foreshadow / hook / aesthetic
	Score     int    `json:"score"`             // 0-100
	Verdict   string `json:"verdict"`           // pass / warning / fail
	Comment   string `json:"comment,omitempty"` // Kết luận ngắn gọn về khía cạnh này
}

// ReviewEntry Bài đánh giá của biên tập viên.
type ReviewEntry struct {
	Chapter          int                `json:"chapter"`
	Scope            string             `json:"scope"` // chapter / global / arc
	Issues           []ConsistencyIssue `json:"issues"`
	Dimensions       []DimensionScore   `json:"dimensions,omitempty"`      // Chấm điểm theo chiều
	ContractStatus   string             `json:"contract_status,omitempty"` // met / partial / missed
	ContractMisses   []string           `json:"contract_misses,omitempty"` // Các hạng mục hợp đồng chưa thực hiện
	ContractNotes    string             `json:"contract_notes,omitempty"`  // Mô tả ngắn gọn về việc thực hiện hợp đồng
	Verdict          string             `json:"verdict"`                   // accept / polish / rewrite
	Summary          string             `json:"summary"`
	AffectedChapters []int              `json:"affected_chapters,omitempty"` // Số chương cần được viết lại/đánh bóng
}

// CriticalCount trả về số lượng vấn đề ở mức độ nghiêm trọng.
func (r *ReviewEntry) CriticalCount() int {
	n := 0
	for _, issue := range r.Issues {
		if issue.Severity == "critical" {
			n++
		}
	}
	return n
}

// ErrorCount trả về số lượng vấn đề về mức độ lỗi.
func (r *ReviewEntry) ErrorCount() int {
	n := 0
	for _, issue := range r.Issues {
		if issue.Severity == "error" {
			n++
		}
	}
	return n
}

// Thứ nguyên Trả về xếp hạng cho thứ nguyên đã chỉ định; trả về con số 0 nếu nó không tồn tại.
func (r *ReviewEntry) Dimension(name string) *DimensionScore {
	if r == nil {
		return nil
	}
	for i := range r.Dimensions {
		if r.Dimensions[i].Dimension == name {
			return &r.Dimensions[i]
		}
	}
	return nil
}
