package utils

import "strings"

// JSONFieldExtractor Trích xuất giá trị chuỗi của một trường được chỉ định từ một đoạn JSON phát trực tuyến.
//
// Khi luồng LLM tạo các lệnh gọi công cụ, các tham số sẽ đến từng mảnh (OpenAI/Anthropic)
// hoặc một cái đến trong một lần (Song Tử). Trình trích xuất này sử dụng máy trạng thái để quét từng ký tự,
// Sau khi phát hiện khóa mục tiêu, hãy trích xuất giá trị chuỗi của nó và xử lý việc thoát JSON.
type JSONFieldExtractor struct {
	key      string // So khớp mục tiêu, chẳng hạn như `"nội dung"` hoặc `"nhiệm vụ"`
	state    extractState
	matchPos int
	escape   bool
	buf      strings.Builder
}

type extractState int

const (
	stateScan    extractState = iota // Quét để tìm khóa mục tiêu
	stateColon                       // Khóa phù hợp, v.v. dấu hai chấm và dấu ngoặc kép mở đầu
	stateExtract                     // Trích xuất giá trị chuỗi
)

func NewFieldExtractor(fieldName string) *JSONFieldExtractor {
	return &JSONFieldExtractor{key: `"` + fieldName + `"`}
}

// Nguồn cấp dữ liệu xử lý một delta và trả về văn bản được trích xuất (có thể trống).
func (e *JSONFieldExtractor) Feed(delta string) string {
	e.buf.Reset()
	for _, r := range delta {
		switch e.state {
		case stateScan:
			e.feedScan(r)
		case stateColon:
			e.feedColon(r)
		case stateExtract:
			e.feedExtract(r)
		}
	}
	return e.buf.String()
}

func (e *JSONFieldExtractor) feedScan(r rune) {
	if e.matchPos < len(e.key) && byte(r) == e.key[e.matchPos] {
		e.matchPos++
		if e.matchPos == len(e.key) {
			e.state = stateColon
			e.matchPos = 0
		}
		return
	}
	e.matchPos = 0
	if byte(r) == e.key[0] {
		e.matchPos = 1
	}
}

func (e *JSONFieldExtractor) feedColon(r rune) {
	switch r {
	case ':', ' ', '\t':
		// nhảy qua
	case '"':
		e.state = stateExtract
		e.escape = false
	default:
		e.state = stateScan
		e.matchPos = 0
		if byte(r) == e.key[0] {
			e.matchPos = 1
		}
	}
}

func (e *JSONFieldExtractor) feedExtract(r rune) {
	if e.escape {
		e.escape = false
		switch r {
		case 'n':
			e.buf.WriteByte('\n')
		case 't':
			e.buf.WriteByte('\t')
		case 'r':
			e.buf.WriteByte('\r')
		case '"', '\\', '/':
			e.buf.WriteRune(r)
		default:
			e.buf.WriteByte('\\')
			e.buf.WriteRune(r)
		}
		return
	}
	switch r {
	case '\\':
		e.escape = true
	case '"':
		e.state = stateScan
		e.matchPos = 0
	default:
		e.buf.WriteRune(r)
	}
}

// Đặt lại trạng thái Đặt lại (được gọi trong vòng tin nhắn LLM mới).
func (e *JSONFieldExtractor) Reset() {
	e.state = stateScan
	e.matchPos = 0
	e.escape = false
}

// Suy nghĩSep là dấu hiệu phân biệt giữa văn bản suy nghĩ và văn bản chính.
// StreamFilter Chèn thẻ này trước khi xem xét phân đoạn văn bản và TUI sẽ chuyển kiểu hiển thị tương ứng.
const ThinkingSep = "\x02"

// StreamFilter phân biệt giữa câu trả lời văn bản của SubAgent và lệnh gọi công cụ JSON.
// Phản hồi văn bản được đánh dấu là nội dung suy nghĩ (tiền tố ThoughtSep); lệnh gọi công cụ JSON chỉ trích xuất các trường được chỉ định.
//
// Cơ sở nhận định: Vào chế độ JSON (theo dõi độ sâu của dấu ngoặc nhọn) khi gặp {
// Trở lại chế độ văn bản sau khi độ sâu được đặt lại về 0.
type StreamFilter struct {
	fieldExt   *JSONFieldExtractor
	mode       filterMode
	braceDepth int
	inString   bool // Trong chuỗi JSON (không tính dấu ngoặc nhọn)
	escJSON    bool // Thoát trong chuỗi JSON
	thinking   bool // Hiện đang trong phần văn bản tư duy
	buf        strings.Builder
}

type filterMode int

const (
	filterText filterMode = iota // Trả lời văn bản, truyền trực tiếp trong suốt
	filterJSON                   // Lệnh gọi công cụ JSON để trích xuất các trường mục tiêu
)

func NewStreamFilter(fieldName string) *StreamFilter {
	return &StreamFilter{fieldExt: NewFieldExtractor(fieldName)}
}

// Nguồn cấp dữ liệu xử lý một vùng delta và trả về văn bản có thể hiển thị.
// Trả lời văn bản được xuất trực tiếp; giá trị trường mục tiêu trong JSON được trích xuất và xuất ra; cấu trúc JSON còn lại bị loại bỏ.
func (f *StreamFilter) Feed(delta string) string {
	f.buf.Reset()
	for _, r := range delta {
		switch f.mode {
		case filterText:
			if r == '{' {
				f.thinking = false
				f.mode = filterJSON
				f.braceDepth = 1
				f.inString = false
				f.escJSON = false
				f.fieldExt.Reset()
				f.feedExtractor(r)
			} else {
				if !f.thinking {
					f.thinking = true
					f.buf.WriteString(ThinkingSep)
				}
				f.buf.WriteRune(r)
			}
		case filterJSON:
			f.feedExtractor(r)
			f.trackBraces(r)
		}
	}
	return f.buf.String()
}

// FeedExtractor cung cấp một ký tự đơn cho fieldExt và ghi kết quả trích xuất vào buf.
func (f *StreamFilter) feedExtractor(r rune) {
	if text := f.fieldExt.Feed(string(r)); text != "" {
		f.buf.WriteString(text)
	}
}

// trackBraces theo dõi độ sâu của dấu ngoặc JSON và chuyển về chế độ văn bản khi độ sâu bằng 0.
func (f *StreamFilter) trackBraces(r rune) {
	if f.escJSON {
		f.escJSON = false
		return
	}
	if f.inString {
		switch r {
		case '\\':
			f.escJSON = true
		case '"':
			f.inString = false
		}
		return
	}
	switch r {
	case '"':
		f.inString = true
	case '{':
		f.braceDepth++
	case '}':
		f.braceDepth--
		if f.braceDepth <= 0 {
			f.mode = filterText
		}
	}
}

// Đặt lại đặt lại trạng thái.
func (f *StreamFilter) Reset() {
	f.mode = filterText
	f.braceDepth = 0
	f.inString = false
	f.escJSON = false
	f.thinking = false
	f.fieldExt.Reset()
}
