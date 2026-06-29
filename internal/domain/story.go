package domain

// Thông tin meta tiểu thuyết tiểu thuyết.
type Novel struct {
	Name          string `json:"name"`
	TotalChapters int    `json:"total_chapters"`
}

// OutlineEntry Mục nhập phác thảo, tương ứng với một chương.
type OutlineEntry struct {
	Chapter   int      `json:"chapter"`
	Title     string   `json:"title"`
	CoreEvent string   `json:"core_event"`
	Hook      string   `json:"hook"`
	Scenes    []string `json:"scenes"`
}

// Hồ sơ nhân vật.
type Character struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"` // Bí danh/chức danh/biệt danh (chẳng hạn như "cậu bé phế vật", "Anh Yan")
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Arc         string   `json:"arc"`
	Traits      []string `json:"traits"`
	Tier        string   `json:"tier,omitempty"` // cốt lõi/quan trọng/thứ yếu/trang trí (mặc định là quan trọng)
}

// VolumeOutline Phác thảo mức âm lượng (chế độ xếp lớp dài).
type VolumeOutline struct {
	Index int          `json:"index"`
	Title string       `json:"title"`
	Theme string       `json:"theme"` // Xung đột/chủ đề cốt lõi của tập này
	Arcs  []ArcOutline `json:"arcs"`
}

// IsExpanded xác định xem âm lượng đã được mở rộng hay chưa (có cấu trúc cấp độ vòng cung).
func (v *VolumeOutline) IsExpanded() bool { return len(v.Arcs) > 0 }

// StoryCompass la bàn hướng kết thúc trò chơi, thay thế danh sách cuộn bộ xương cố định.
// Kiến trúc sư cập nhật ở mỗi ranh giới cuộn, cho phép hướng câu chuyện phát triển cùng với sự sáng tạo.
type StoryCompass struct {
	EndingDirection string   `json:"ending_direction"`          // Hướng kết thúc (mô tả chuyên đề)
	OpenThreads     []string `json:"open_threads,omitempty"`    // Hoạt động lâu dài (cần đóng cửa đến hết)
	EstimatedScale  string   `json:"estimated_scale,omitempty"` // Thang đo mờ (ví dụ: "Dự kiến ​​4-6 tập")
	LastUpdated     int      `json:"last_updated,omitempty"`    // Số chương đã hoàn thành tại thời điểm cập nhật
}

// Đường viền cấp độ cung ArcOutline.
type ArcOutline struct {
	Index             int            `json:"index"` // Số hồ quang bên trong khối lượng
	Title             string         `json:"title"`
	Goal              string         `json:"goal"`                         // Mục tiêu vòng cung (chuyển tiếp ban đầu)
	EstimatedChapters int            `json:"estimated_chapters,omitempty"` // Số chương ước tính của cốt truyện (xóa sau khi mở rộng)
	Chapters          []OutlineEntry `json:"chapters"`
}

// IsExpanded xác định xem cung đã được mở rộng hay chưa (chương chi tiết).
func (a *ArcOutline) IsExpanded() bool { return len(a.Chapters) > 0 }

// TotalChapters Tính toán tổng số chương theo kế hoạch hiện tại trong sơ đồ phân cấp.
// Các cung mở rộng được tính là các chương thực và các cung khung được tính là các Chương ước tính.
// Progress.TotalChapters sử dụng nó để xác định chiến lược bối cảnh dài hạn; các chương thực sự có thể ghi được vẫn đến từ FlattenOutline.
func TotalChapters(volumes []VolumeOutline) int {
	n := 0
	for _, v := range volumes {
		for _, a := range v.Arcs {
			if a.IsExpanded() {
				n += len(a.Chapters)
			} else {
				n += a.EstimatedChapters
			}
		}
	}
	return n
}

// FlattenOutline Mở rộng dàn ý phân cấp thành một danh sách phẳng các chương, giữ cho số chương toàn cầu được liên tục.
func FlattenOutline(volumes []VolumeOutline) []OutlineEntry {
	var result []OutlineEntry
	ch := 1
	for _, v := range volumes {
		for _, a := range v.Arcs {
			for _, e := range a.Chapters {
				e.Chapter = ch
				result = append(result, e)
				ch++
			}
		}
	}
	return result
}

// Mục nhập quy tắc thế giới quan của WorldRule.
type WorldRule struct {
	Category string `json:"category"` // magic / technology / geography / society / other
	Rule     string `json:"rule"`     // Mô tả quy tắc
	Boundary string `json:"boundary"` // ranh giới bất khả xâm phạm
}
