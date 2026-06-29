package host

import (
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/store"
)

// buildStoryStateSummary tập hợp một bản tóm tắt cô đọng về trạng thái hiện tại của câu chuyện để trợ lý đồng sáng tạo sân khấu hiểu "những gì đã được viết".
// Tái sử dụng các điểm truy cập lưu trữ và chỉ tìm nạp các thông tin cấp cao cần thiết cho việc định hướng lập kế hoạch (tiến trình/la bàn/tập gần đây/nhân vật chính/điềm báo đang hoạt động);
// Không kéo văn bản chính, không cung cấp JSON đầy đủ của tiểu thuyết_context - đồng sáng tạo là một cuộc đối thoại, điều cần thiết là một cái nhìn tổng quan có thể đọc được chứ không phải bối cảnh viết.
// Nếu bất kỳ mục nào bị thiếu, nó sẽ bị bỏ qua (nỗ lực tốt nhất) và một chuỗi trống sẽ được trả về để cho biết rằng không có tiến trình nào.
func buildStoryStateSummary(s *store.Store) string {
	if s == nil {
		return ""
	}
	var b strings.Builder

	if progress, _ := s.Progress.Load(); progress != nil {
		if name := strings.TrimSpace(progress.NovelName); name != "" {
			fmt.Fprintf(&b, "- Tên sách: “%s” \n", name)
		}
		fmt.Fprintf(&b, "- Tiến độ: Đã hoàn thành chương %d", len(progress.CompletedChapters))
		if progress.TotalChapters > 0 {
			fmt.Fprintf(&b, " / Quy hoạch Chương %d", progress.TotalChapters)
		}
		fmt.Fprintf(&b, ", về từ %d, chương tiếp theo là Chương %d \n", progress.TotalWordCount, progress.NextChapter())
		if progress.Layered && progress.CurrentVolume > 0 {
			fmt.Fprintf(&b, "- Vị trí hiện tại: Tập %d Chương %d Arc \n", progress.CurrentVolume, progress.CurrentArc)
		}
	}

	if compass, _ := s.Outline.LoadCompass(); compass != nil {
		if dir := strings.TrimSpace(compass.EndingDirection); dir != "" {
			fmt.Fprintf(&b, "- Hướng cuối cùng: %s\n", dir)
		}
		if compass.EstimatedScale != "" {
			fmt.Fprintf(&b, "- Kích thước dự kiến: %s\n", compass.EstimatedScale)
		}
		if len(compass.OpenThreads) > 0 {
			fmt.Fprintf(&b, "- Hoạt động lâu dài: %s\n", strings.Join(compass.OpenThreads, "；"))
		}
	}

	// Tóm tắt tập gần đây nhất, cho trợ lý biết câu chuyện vừa đi đến đâu
	if vols, _ := s.Summaries.LoadAllVolumeSummaries(); len(vols) > 0 {
		last := vols[len(vols)-1]
		fmt.Fprintf(&b, "- \"%s\" gần đây nhất: %s\n", last.Title, truncate(last.Summary, 200))
	}

	// Ký tự chính (cốt lõi/quan trọng), tối đa 8
	if chars, _ := s.Characters.Load(); len(chars) > 0 {
		var names []string
		for _, c := range chars {
			if c.Tier == "secondary" || c.Tier == "decorative" {
				continue
			}
			line := c.Name
			if role := strings.TrimSpace(c.Role); role != "" {
				line += "（" + role + "）"
			}
			names = append(names, line)
			if len(names) >= 8 {
				break
			}
		}
		if len(names) > 0 {
			fmt.Fprintf(&b, "- Nhân vật chính: %s\n", strings.Join(names, "、"))
		}
	}

	// Điềm báo không được thu thập, tối đa 6
	if fs, _ := s.World.LoadActiveForeshadow(); len(fs) > 0 {
		var items []string
		for _, f := range fs {
			items = append(items, truncate(f.Description, 40))
			if len(items) >= 6 {
				break
			}
		}
		fmt.Fprintf(&b, "- Gợi ý chưa nhận được: %s\n", strings.Join(items, "；"))
	}

	return strings.TrimSpace(b.String())
}

// stageSystemPrompt Tập hợp lời nhắc hệ thống hoàn chỉnh để đồng tạo giai đoạn: lời nhắc giai đoạn + tóm tắt trạng thái câu chuyện hiện tại.
// Bản tóm tắt được treo ở cuối dưới dạng phụ lục dữ liệu (được phân tách bằng dòng phân cách và đặc tả định dạng), lặp lại hướng dẫn trong lời nhắc "xem tiến trình bên dưới".
func stageSystemPrompt(s *store.Store) string {
	prompt := stageCoCreateSystemPrompt
	if summary := buildStoryStateSummary(s); summary != "" {
		prompt += "\n\n---\n## Trạng thái truyện hiện tại \n (Sau đây là tóm tắt khách quan những gì đã viết, để các bạn tham khảo khi lập kế hoạch theo dõi, không sao chép nguyên văn <dự thảo>) \n" + summary
	}
	return prompt
}
