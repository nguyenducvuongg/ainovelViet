// Gói stylestat thực hiện thống kê kiểu cấp độ sách trên văn bản viết và tạo ra các dữ kiện thuần túy.
//
// Động lực: Cửa sổ ôn tập trong phần (~10 chương) củng cố sự mù quáng tự nhiên của toàn bộ mô hình cấp độ cuốn sách - các mẫu câu và chương có hàng chục lần,
// Hình thức ở cuối chương là đẳng hình và khi đọc qua các chương, mọi thứ đều có vẻ "bình thường" trong một chương duy nhất và chỉ có số liệu thống kê của cả cuốn sách mới có thể tiết lộ điều đó. Mã thống kê
//(Chắc chắn, không ảo tưởng), phán quyết thuộc về LLM (người biên tập đánh giá các kích thước dựa trên các con số và người viết tránh né chính mình theo đó).
package stylestat

import (
	"regexp"
	"sort"
	"strings"
)

// Không thể tính các Chương tối thiểu ít hơn chương này - mẫu quá nhỏ và tần suất là vô nghĩa.
const minChapters = 5

// PhraseWindow khai thác cụm từ động chỉ xem xét N chương mới nhất: Điều người viết cần tránh là "câu thần chú hiện tại".
const phraseWindow = 20

// Nhập dữ liệu thống kê đầu vào. Các chương được sắp xếp tăng dần theo số chương; Từ dừng là những danh từ riêng như tên nhân vật.
// Bỏ qua khi khai thác các cụm từ động (tên của những người xuất hiện tự nhiên có tần suất cao, không phải vấn đề về phong cách viết).
type Input struct {
	Chapters  []string
	Titles    []string
	Stopwords []string
}

// Kết quả thống kê theo kiểu sách thống kê. Tất cả các trường đều là số liệu thực tế và không chứa bất kỳ phán quyết hoặc chỉ thị nào.
type Stats struct {
	Chapters          int            `json:"chapters"`
	Patterns          []PatternStat  `json:"patterns,omitempty"`
	TopPhrases        []PhraseStat   `json:"top_phrases,omitempty"`
	RepeatedSentences []SentenceStat `json:"repeated_sentences,omitempty"`
	Ending            EndingStat     `json:"ending"`
	OpeningTimeRate   float64        `json:"opening_time_rate"`
	TitleFormats      *TitleStat     `json:"title_formats,omitempty"`
}

// PatternStat là tổng số sách của các lớp mẫu câu cố định (kiểu viết AI chung tic).
type PatternStat struct {
	Name       string  `json:"name"`
	Total      int     `json:"total"`
	PerChapter float64 `json:"per_chapter"`
}

// PhraseStat Các cụm từ tần số cao được khai thác gần đây trong chương PhraseWindow.
type PhraseStat struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// SentenceStat Những câu dài được lặp lại nguyên văn trong các chương (bằng chứng trực tiếp về việc đọc lại).
type SentenceStat struct {
	Text     string `json:"text"`
	Chapters int    `json:"chapters"`
	Count    int    `json:"count"`
}

// Phân phối mẫu dòng cuối chương EndingStat. Bản thân phần kết thúc ngắn là hợp pháp, chính sự đồng hình của toàn bộ cuốn sách mới là vấn đề.
type EndingStat struct {
	ShortRatio  float64 `json:"short_ratio"`
	MedianRunes int     `json:"median_runes"`
}

// Số lượng hỗn hợp tiền tố TitleStat Tiêu đề Chương "Chương N" (hỗn hợp = dấu vết cơ học lộ ra trong sản phẩm).
type TitleStat struct {
	WithPrefix    int `json:"with_prefix"`
	WithoutPrefix int `json:"without_prefix"`
}

// sampleDefs là một mẫu câu kiểu AI chung. Đếm là gần đúng (biểu thức chính quy không được phân tích cú pháp),
// Mục đích là so sánh đường cơ sở theo chiều dọc của chính cuốn sách và độ chính xác tuyệt đối không quan trọng.
var patternDefs = []struct {
	name string
	re   *regexp.Regexp
}{
	{"Cấu trúc câu \"không phải... mà là...\"", regexp.MustCompile(`(?:không|chẳng)\s+phải\s+[^.!?\n]{1,24}?\s+mà\s+(?:là|phải)`)},
	{"Bộ định lượng thời gian \"X hơi thở/X tích tắc\"", regexp.MustCompile(`(?:vài|mấy|một|hai|ba|bốn|năm|sáu|bảy|tám|chín|mười|nửa|\d+)\s*(?:hơi thở|nhịp thở|tích tắc|chớp mắt|giây|khoảnh khắc|phút)`)},
	{"So sánh \"giống như/tựa như/như thể\"", regexp.MustCompile(`(?:giống như|tựa như|như thể)\s+[^.!?\n]{1,16}`)},
	{"Nhịp im lặng \"im lặng/không nói/không quay lại\"", regexp.MustCompile(`im lặng|không nói|không trả lời|không quay đầu|không quay lại|quay người`)},
}

var (
	sentenceSplit = regexp.MustCompile(`[。！？\n]+`)
	openingTimeRe = regexp.MustCompile(`đêm|buổi sáng|bình minh|rạng đông|thức dậy|cả đêm`)
	titlePrefixRe = regexp.MustCompile(`(?i)^#{0,2}\s*(?:chương\s+\d+|chapter\s+\d+)`)
)

// shortEndingRunes Dòng cuối cùng không vượt quá số từ này và được tính là "kết thúc ngắn".
const shortEndingRunes = 30

// Tính toán Thống kê kiểu sách; trả về con số 0 nếu không có đủ chương.
func Compute(in Input) *Stats {
	n := len(in.Chapters)
	if n < minChapters {
		return nil
	}
	all := strings.Join(in.Chapters, "\n")

	s := &Stats{Chapters: n}
	for _, def := range patternDefs {
		total := len(def.re.FindAllStringIndex(all, -1))
		if total == 0 {
			continue
		}
		s.Patterns = append(s.Patterns, PatternStat{
			Name:       def.name,
			Total:      total,
			PerChapter: round1(float64(total) / float64(n)),
		})
	}
	s.TopPhrases = minePhrases(recentWindow(in.Chapters), in.Stopwords)
	s.RepeatedSentences = repeatedSentences(in.Chapters)
	s.Ending = endingShape(in.Chapters)
	s.OpeningTimeRate = openingTimeRate(in.Chapters)
	s.TitleFormats = titleFormats(in.Titles)
	return s
}

func recentWindow(chapters []string) []string {
	if len(chapters) <= phraseWindow {
		return chapters
	}
	return chapters[len(chapters)-phraseWindow:]
}

// MinePhrases khai thác các cụm từ tần số cao 3-6 từ trong một cửa sổ.
// Lọc: bao gồm dấu câu/khoảng trống, từ chức năng đầu và cuối, nhấn danh từ riêng; loại bỏ trùng lặp: loại bỏ những chuỗi con của cụm từ đã chọn.
func minePhrases(chapters []string, stopwords []string) []PhraseStat {
	text := strings.Join(chapters, "\n")
	runes := []rune(text)
	threshold := max(8, len(chapters)/2)

	counts := make(map[string]int)
	for size := 3; size <= 6; size++ {
		for i := 0; i+size <= len(runes); i++ {
			gram := runes[i : i+size]
			if !validGram(gram) {
				continue
			}
			counts[string(gram)]++
		}
	}

	stopGrams := stopwordBigrams(stopwords)
	type cand struct {
		text  string
		count int
	}
	var cands []cand
	for g, c := range counts {
		if c < threshold || hitStopword(g, stopGrams) {
			continue
		}
		cands = append(cands, cand{g, c})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].count != cands[j].count {
			return cands[i].count > cands[j].count
		}
		// Lấy cái dài hơn với cùng tần suất (lượng thông tin lớn hơn), sau đó sắp xếp nó ổn định theo thứ tự từ điển.
		if len(cands[i].text) != len(cands[j].text) {
			return len(cands[i].text) > len(cands[j].text)
		}
		return cands[i].text < cands[j].text
	})

	var out []PhraseStat
	for _, c := range cands {
		if len(out) >= 8 {
			break
		}
		dup := false
		for _, picked := range out {
			if strings.Contains(picked.Text, c.text) || strings.Contains(c.text, picked.Text) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, PhraseStat{Text: c.text, Count: c.count})
		}
	}
	return out
}

// gramEdgeStop n-gram bắt đầu và kết thúc bằng các từ/đại từ chức năng này không phải là cụm từ mang tính văn phong, vì vậy hãy bỏ qua chúng.
const gramEdgeStop = "Hơn nữa, tất cả đều là về anh ấy, cô ấy, tôi, bạn, cái này, cái kia."

func validGram(gram []rune) bool {
	for _, r := range gram {
		if r < 0x4E00 || r > 0x9FFF { // Chỉ có những mảnh ký tự thuần Trung Quốc
			return false
		}
	}
	if strings.ContainsRune(gramEdgeStop, gram[0]) || strings.ContainsRune(gramEdgeStop, gram[len(gram)-1]) {
		return false
	}
	return true
}

// stopwordBigrams Chia danh từ riêng thành các đoạn 2 từ: tên cá nhân thường được đưa vào văn bản ở dạng một phần
//("Jiuyuan chắp tay sau lưng" chứa "Jiuyuan"). Ghép theo tên đầy đủ sẽ bị mất dấu. Thích lọc nghiêm ngặt - ít cụm từ và dữ kiện hơn
// Không vấn đề gì, những cái tên trộn lẫn vào danh sách các câu thần chú chỉ là tiếng ồn.
func stopwordBigrams(stopwords []string) []string {
	var grams []string
	for _, w := range stopwords {
		runes := []rune(strings.TrimSpace(w))
		if len(runes) < 2 {
			continue
		}
		for i := 0; i+2 <= len(runes); i++ {
			grams = append(grams, string(runes[i:i+2]))
		}
	}
	return grams
}

func hitStopword(gram string, stopGrams []string) bool {
	for _, g := range stopGrams {
		if strings.Contains(gram, g) {
			return true
		}
	}
	return false
}

// lặp lạiSentences tìm thấy các câu có ≥12 từ được lặp lại nguyên văn trong ≥3 chương và chọn 5 câu hàng đầu theo tần suất.
func repeatedSentences(chapters []string) []SentenceStat {
	type rec struct {
		count    int
		chapters map[int]struct{}
	}
	seen := make(map[string]*rec)
	for ci, text := range chapters {
		for _, sent := range sentenceSplit.Split(text, -1) {
			// Tách các dấu ngoặc kép và hợp nhất chúng lại với nhau: cùng một dòng có hoặc không có dấu ngoặc kép trước đó không được tính là hai
			sent = strings.Trim(strings.TrimSpace(sent), `"“”‘’「」『』`)
			if len([]rune(sent)) < 12 {
				continue
			}
			r := seen[sent]
			if r == nil {
				r = &rec{chapters: make(map[int]struct{})}
				seen[sent] = r
			}
			r.count++
			r.chapters[ci] = struct{}{}
		}
	}

	var out []SentenceStat
	for sent, r := range seen {
		if len(r.chapters) < 3 {
			continue
		}
		out = append(out, SentenceStat{Text: truncateRunes(sent, 40), Chapters: len(r.chapters), Count: r.count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Text < out[j].Text
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func endingShape(chapters []string) EndingStat {
	var lengths []int
	short := 0
	for _, text := range chapters {
		line := lastNonEmptyLine(text)
		if line == "" {
			continue
		}
		n := len([]rune(line))
		lengths = append(lengths, n)
		if n <= shortEndingRunes {
			short++
		}
	}
	if len(lengths) == 0 {
		return EndingStat{}
	}
	sort.Ints(lengths)
	return EndingStat{
		ShortRatio:  round2(float64(short) / float64(len(lengths))),
		MedianRunes: lengths[len(lengths)/2],
	}
}

func openingTimeRate(chapters []string) float64 {
	hit := 0
	for _, text := range chapters {
		if openingTimeRe.MatchString(firstParagraph(text)) {
			hit++
		}
	}
	return round2(float64(hit) / float64(len(chapters)))
}

func titleFormats(titles []string) *TitleStat {
	if len(titles) == 0 {
		return nil
	}
	t := &TitleStat{}
	for _, title := range titles {
		if strings.TrimSpace(title) == "" {
			continue
		}
		if titlePrefixRe.MatchString(title) {
			t.WithPrefix++
		} else {
			t.WithoutPrefix++
		}
	}
	// Chỉ có mục đích sử dụng hỗn hợp mới đáng báo cáo; định dạng thống nhất không phải là vấn đề thực tế
	if t.WithPrefix == 0 || t.WithoutPrefix == 0 {
		return nil
	}
	return t
}

func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// firstParagraph lấy dòng đầu tiên không trống và không có tiêu đề Markdown (dòng đầu tiên của tệp chương luôn là tiêu đề #).
func firstParagraph(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
