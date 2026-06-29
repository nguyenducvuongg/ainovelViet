package imp

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/nguyenducvuongg/ainovelViet/internal/utils"
)

// Chuẩn hóa tiêu đề chương mặc định. Bao gồm tiếng Trung thông dụng (Chương N/chương/tập/tập/phần/hành động, tập N, mở đầu/nêm/kết thúc/ngoại truyện/truyện bên lề, v.v.)
// Tương thích với các tiêu đề tiếng Anh (Chương N, Mở đầu, Kết thúc), tiền tố tiêu đề Markdown (# / ##),
// Điểm bắt đầu là tiền tố "Chương N" của txt và tiêu đề của gói []〖〗.
//
// Nhóm được đặt tên: Các nhóm phụ đề được ưu tiên hơn các nhóm từ khóa (phục hồi theo thứ tự ưu tiên khi giải nén):
//   - cn phụ đề chương được đánh số (văn bản sau Chương X/Chương/Tập/Tập/Phần/Đạo)
//   - phụ đề tập độc lập vol (văn bản sau Tập X)
//   - sp phụ đề đơn vị đặc biệt (văn bản sau phần mở đầu/nêm/phần kết/bổ sung)
//   - vi Phụ đề chương tiếng Anh (văn bản sau Chương X / Mở đầu / Kết thúc)
//   - spkw Bản thân từ khóa đơn vị đặc biệt (sử dụng tiêu đề khi không có phụ đề, chẳng hạn như "nêm" và "thêm")
//   - enkw Bản thân từ khóa đơn vị đặc biệt tiếng Anh (sử dụng tiêu đề khi không có phụ đề, chẳng hạn như "Mở đầu")

// ws là nội dung ký tự: ASCII trống + khoảng trắng toàn chiều rộng. \s của Go RE2 chỉ chứa khoảng trắng ASCII,
// Cách phân tách tiêu đề thường được sử dụng trong sắp chữ tiếng Trung là U+3000 ("Chương 1: Gió Nổi").
const ws = `\s\x{3000}`

// cnNum là các ký tự số có sẵn cho số chương: tiếng Ả Rập/độ rộng đầy đủ/chữ thường tiếng Trung/chữ hoa tiếng Trung truyền thống (một hai mươi ba...mười nghìn).
const cnNum = `零〇○Ｏ０一二三四五六七八九十百千万两壹贰貳叁參肆伍陆陸柒捌玖拾佰仟萬兩\d`

// sub là chụp phụ đề: nó đến cuối dòng, nhưng không nuốt phần bao bên phải (]〗), để lại dấu ngoặc đóng tùy chọn ở cuối.
const sub = `[^】〗\n]*`

var defaultChapterRegex = regexp.MustCompile(
	`(?im)^#{0,2}[` + ws + `]*(?:正文|chữ|Chính văn[` + ws + `]*)?[【〖]?[` + ws + `]*(?:` +
		`(?:第|Chương|Chương\s+số|Chương\s+|Chapter|Không\.)\s*(?:[` + cnNum + `]+)\s*(?:章|回|话|卷|节|幕|Chương|chương|tập|phần|đoạn)` +
		`(?:[:：．\.` + ws + `-]+(?P<cn>` + sub + `))?` +
		`|` +
		`(?:卷|Tập|Volume)\s*(?:[` + cnNum + `]+)` +
		`(?:[:：．\.` + ws + `-]+(?P<vol>` + sub + `))?` +
		`|` +
		`(?P<spkw>序章|序幕|楔子|引子|前言|序言|尾声|终章|后记|番外|外传|Mở đầu|Mở ​​đầu|Nêm|Giới thiệu|Lời nói đầu|Lời nói đầu|Phần kết|Chương cuối|Bài viết|Thêm|Thêm|Ngoại truyện)` +
		`(?:[:：．\.` + ws + `-]+(?P<sp>` + sub + `))?` +
		`|` +
		`(?:Chapter\s+(?:\d+|[IVXLCDM]+)|(?P<enkw>Prologue|Epilogue))` +
		`(?:[:：．\.` + ws + `-]+(?P<en>` + sub + `))?` +
		`)[` + ws + `]*[】〗]?[` + ws + `]*$`,
)

// SplitFile chia một tệp văn bản thành một danh sách các chương.
func SplitFile(path string) ([]Chapter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	text := utils.DecodeText(data)
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("source file is empty: %s", path)
	}
	return splitText(text, defaultChapterRegex), nil
}

// SplitText là một phiên bản phân tách chức năng thuần túy, thuận tiện cho việc thử nghiệm đơn lẻ.
func splitText(text string, pattern *regexp.Regexp) []Chapter {
	lines := strings.Split(text, "\n")
	type marker struct {
		line  int
		title string
	}
	var marks []marker
	for i, ln := range lines {
		if loc := pattern.FindStringSubmatchIndex(ln); loc != nil {
			marks = append(marks, marker{line: i, title: extractTitle(ln, pattern, loc, len(marks)+1)})
		}
	}
	if len(marks) == 0 {
		return nil
	}

	chapters := make([]Chapter, 0, len(marks))
	for i, m := range marks {
		end := len(lines)
		if i+1 < len(marks) {
			end = marks[i+1].line
		}
		body := strings.Join(lines[m.line+1:end], "\n")
		body = stripTrailingNoise(body)
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		chapters = append(chapters, Chapter{Title: m.title, Content: body})
	}
	return chapters
}

// extractTitle trích xuất tiêu đề chương từ dòng phù hợp; ưu tiên lấy bản chụp được đặt tên, nếu không thì quay lại phần giữ chỗ số chương.
func extractTitle(line string, pattern *regexp.Regexp, loc []int, fallbackNum int) string {
	subnames := pattern.SubexpNames()
	priority := []string{"cn", "vol", "sp", "en", "spkw", "enkw"}
	for _, name := range priority {
		idx := pattern.SubexpIndex(name)
		if idx <= 0 {
			continue
		}
		if loc[2*idx] < 0 {
			continue
		}
		if t := strings.TrimSpace(line[loc[2*idx]:loc[2*idx+1]]); t != "" {
			return t
		}
	}
	// Tìm hiểu phần cuối của nó: chiếm nhóm chụp không trống đầu tiên (phòng thủ, nhóm có tên thông thường mặc định bao gồm tất cả các nhánh)
	for i := 1; i < len(subnames); i++ {
		if loc[2*i] < 0 {
			continue
		}
		if t := strings.TrimSpace(line[loc[2*i]:loc[2*i+1]]); t != "" {
			return t
		}
	}
	if strings.Contains(line, "第") {
		return fmt.Sprintf("第%d章", fallbackNum)
	}
	return fmt.Sprintf("Chương %d", fallbackNum)
}

// dảiTrailingNoise Loại bỏ tiếng ồn đuôi phổ biến (Đoạn giới thiệu giấy phép của Project Gutenberg và cộng sự).
var trailerRe = regexp.MustCompile(`(?im)^\s*Project Gutenberg(?:\(TM\)|™)?[\s\S]*$`)

func stripTrailingNoise(content string) string {
	if loc := trailerRe.FindStringIndex(content); loc != nil {
		return strings.TrimRight(content[:loc[0]], " \t\n")
	}
	return content
}
