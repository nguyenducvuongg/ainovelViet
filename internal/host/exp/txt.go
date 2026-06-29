package exp

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// chươngTitleIndex tìm kiếm tiêu đề theo số chương, nếu thiếu, trả về một chuỗi trống.
type chapterTitleIndex map[int]string

func buildTitleIndex(outline []domain.OutlineEntry) chapterTitleIndex {
	idx := make(chapterTitleIndex, len(outline))
	for _, e := range outline {
		if e.Title != "" {
			idx[e.Chapter] = e.Title
		}
	}
	return idx
}

// chươngVị trí là sự ghi công của một chương trong dàn ý phân cấp. Chỉ giữ lại thông tin ổ đĩa cần thiết cho định dạng xuất——
// Các cung không vào hoặc ra (các cung là các cấu trúc bên trong quá mỏng theo góc nhìn của người đọc).
type chapterLocation struct {
	VolumeIdx       int
	VolumeTitle     string
	IsFirstOfVolume bool
}

// buildLocations xây dựng {chương -> vị trí} theo thứ tự chương chung của phác thảo phân cấp.
// Số chương được xây dựng lại theo quy tắc tương tự như FlattenOutline (tích lũy tuần tự trong các tập và cung),
// Để giữ số chương nhất quán với Progress.CompletedChapters. Lớp cung vẫn cần được duyệt qua (cần tính số chương toàn cầu),
// Nhưng không phải ở vị trí - việc xuất chỉ chèn dấu phân cách ở đầu tập.
func buildLocations(volumes []domain.VolumeOutline) map[int]chapterLocation {
	if len(volumes) == 0 {
		return nil
	}
	locs := make(map[int]chapterLocation)
	ch := 0
	for _, v := range volumes {
		firstOfVol := true
		for _, a := range v.Arcs {
			for range a.Chapters {
				ch++
				locs[ch] = chapterLocation{
					VolumeIdx:       v.Index,
					VolumeTitle:     v.Title,
					IsFirstOfVolume: firstOfVol,
				}
				firstOfVol = false
			}
		}
	}
	return locs
}

// chươngHeaderRe khớp dòng đầu tiên của tiêu đề Markdown với số chương (# Chương N / ## Chương 12...).
var chapterHeaderRe = regexp.MustCompile(`^#+\s+(?:Chương|第|Chapter).+?`)

// atxTitleRe Trích xuất phần văn bản của tiêu đề ATX (# tiêu đề).
var atxTitleRe = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*$`)

// StripChapterTitleHeader Nếu dòng đầu tiên là tiêu đề chương sẽ trùng lặp với tiêu đề thống nhất của nhà xuất khẩu, hãy loại bỏ nó.
// Hai tình huống: ① "# Chương N..." (có số chương); ② tiêu đề đánh dấu và văn bản của nó chính xác là tiêu đề của chương này
// (Người viết thường viết tên chương thuần túy làm tiêu đề vào dòng đầu tiên của văn bản, chẳng hạn như "#cuộc sống nổi làng biên giới", giống với tên do nhà xuất khẩu tạo ra.
// "Chương N: Cuộc sống trôi nổi ở làng biên giới" lặp lại). Các h1 khác (chẳng hạn như "#prologue") được coi là một phần của văn bản chính và được giữ lại.
// Người gọi chịu trách nhiệm về TrimSpace trước nên các dòng trống ở đầu sẽ không được xem xét.
func stripChapterTitleHeader(content, title string) string {
	first, rest, hasNewline := strings.Cut(content, "\n")
	if !isChapterTitleLine(first, title) {
		return content
	}
	if !hasNewline {
		return ""
	}
	return strings.TrimLeft(rest, "\n")
}

func isChapterTitleLine(line, title string) bool {
	if chapterHeaderRe.MatchString(line) {
		return true
	}
	if title = strings.TrimSpace(title); title == "" {
		return false
	}
	m := atxTitleRe.FindStringSubmatch(line)
	return len(m) == 2 && strings.TrimSpace(m[1]) == title
}

// renderTXT nối văn bản cuối cùng.
//
// Thứ tự của các chương được xác định theo các chương (người gọi đã loại bỏ chúng theo thứ tự tăng dần của số chương). nội dung/Idx tiêu đề/vị trí
// Tất cả đều được coi là "thiếu có nghĩa là bị hạ cấp": nếu thiếu tiêu đề, chỉ xuất ra "Chương N"; nếu thiếu vị trí phân cấp, nó sẽ được coi là đường viền phẳng.
func renderTXT(
	novelName string,
	chapters []int,
	titleIdx chapterTitleIndex,
	locations map[int]chapterLocation,
	bodies map[int]string,
) string {
	var b strings.Builder

	if name := strings.TrimSpace(novelName); name != "" {
		b.WriteString("《")
		b.WriteString(name)
		b.WriteString("》\n\n")
	}

	useLayered := len(locations) > 0

	for i, ch := range chapters {
		if useLayered {
			if loc, ok := locations[ch]; ok && loc.IsFirstOfVolume {
				b.WriteString("\n═══════════════════════════════════════════\n")
				fmt.Fprintf(&b, "           Tập %d %s\n", loc.VolumeIdx, strings.TrimSpace(loc.VolumeTitle))
				b.WriteString("═══════════════════════════════════════════\n\n")
			}
		}

		title := strings.TrimSpace(titleIdx[ch])
		if title != "" {
			fmt.Fprintf(&b, "Chương %d %s\n\n", ch, title)
		} else {
			fmt.Fprintf(&b, "Chương %d\n\n", ch)
		}

		body := stripChapterTitleHeader(strings.TrimSpace(bodies[ch]), title)
		b.WriteString(body)
		b.WriteString("\n")
		if i < len(chapters)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}
