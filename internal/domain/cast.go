package domain

// CastEntry là một kỷ lục về diễn viên phụ trong danh sách diễn viên phụ.
//
// Tách khỏi Ký tự (characters.json, tệp lõi được duy trì bởi Architect):
//   - CastEntry được công cụ commit_chapter tự động tích lũy để ghi lại "các ký tự phụ có tên đã xuất hiện"
//   - Nhân vật được Kiến trúc sư thiết kế rõ ràng nhằm ghi lại các cung/đặc điểm/cấp độ tính cách của nhân vật chính và các nhân vật phụ chủ chốt
//
// Khi sử dụng cùng tên, Ký tự sẽ được ưu tiên (các ký tự cốt lõi sẽ không được đưa vào cast_ledger) để tránh trùng lặp.
type CastEntry struct {
	Name string `json:"name"`
	// Bí danh hiện không có kênh ghi; dành riêng cho công cụ "bí danh hợp nhất chỉ đạo người dùng" trong tương lai
	// (Ví dụ: tuyên bố 'Chủ quán Li' và 'Lão Li' là cùng một người). MergeAppearances đã hỗ trợ tra cứu bí danh.
	Aliases          []string `json:"aliases,omitempty"`
	BriefRole        string   `json:"brief_role,omitempty"` // Định vị một câu (Người viết điền vào lần xuất hiện đầu tiên, có thể điền sau; sẽ không bị ghi đè)
	FirstSeenChapter int      `json:"first_seen_chapter"`
	LastSeenChapter  int      `json:"last_seen_chapter"`
	// AppearanceCount có nguồn gốc từ len(AppearanceChapters) và vẫn được đồng bộ hóa khi hợp nhất.
	// Các trường rõ ràng được dành riêng để UI/JSON đọc trực tiếp mà không cần phải tính toán lại mỗi lần.
	AppearanceCount    int   `json:"appearance_count"`
	AppearanceChapters []int `json:"appearance_chapters"`
	// Đã thăng hạng đánh dấu mục này là được thăng cấp lên character.json. Hoạt động gần đây bỏ qua các mục này,
	// Tránh thu hồi trùng lặp với các kho lưu trữ cốt lõi. Kênh nâng cấp hiện tại không được triển khai và trường này là một hook dành riêng.
	Promoted bool `json:"promoted,omitempty"`
}

// CastIntro là lời giới thiệu của Người viết dành cho nhân vật mới khi commit_chapter.
// Chỉ được lấy nếu tên xuất hiện lần đầu tiên hoặc nếu BriefRole vẫn trống trong sổ cái.
type CastIntro struct {
	Name      string `json:"name"`
	BriefRole string `json:"brief_role"`
}
