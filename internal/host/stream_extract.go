package host

import (
	"strings"
	"unicode/utf8"
)

// toolDisplays định cấu hình chiến lược hiển thị của từng công cụ trên bảng điều khiển quy trình. Các công cụ không có trong bảng này không tham gia phát trực tuyến
// Hiển thị (người quan sát loại bỏ DeltaToolCall trực tiếp).
//
// Chế độ chung (nakedKey trống): trình mã thông báo hiển thị đầu ra JSON của đối số bằng LLM thành định dạng thụt lề
// văn bản "khóa: giá trị", các đối tượng/mảng lồng nhau được thụt lề theo thứ bậc, đầu ra phát trực tuyến chuỗi/số/bool.
// Tách hoàn toàn khỏi lược đồ - nếu LLM xuất ra thêm một trường, điều đó có nghĩa là có thêm một dòng trên bảng mà không có bất kỳ thay đổi mã nào.
//
// Chế độ luồng trần (nakedKey không trống): Chỉ giá trị chuỗi của trường cấp cao nhất mục tiêu được truyền phát nguyên trạng và các trường khác được truyền phát
// Bỏ qua tất cả. Được sử dụng bởi Draft_chapter để ngăn chặn việc đánh dấu toàn bộ chương là "nội dung: # ...".
// Tiêu đề luôn bắt đầu bằng "✻": đây là TUI renderStreamContent sử dụng renderAgentBlock
// Tiền tố đã thống nhất cho đường dẫn được đánh dấu (vàng ✻ + nhãn gạch chân màu xanh lam + đường ngang mờ), theo sau là dự phòng
// tiêu đề (streamHeaderFallback) vẫn nhất quán; thay đổi thành văn bản bình thường sẽ rơi vào đường dẫn văn bản bằng thiết bị đầu cuối
// Màu mặc định bị nhạt đi và tiêu đề không còn bắt mắt nữa.
var toolDisplays = map[string]toolDisplay{
	"draft_chapter": {nakedKey: "content"},

	"plan_chapter":        {header: "✻ Lập kế hoạch"},
	"edit_chapter":        {header: "✻ Đánh bóng"},
	"commit_chapter":      {header: "✻ Gửi chương"},
	"save_review":         {header: "✻ Đánh giá"},
	"save_arc_summary":    {header: "✻ Tóm tắt phần"},
	"save_volume_summary": {header: "✻ Tóm tắt tập"},
	"save_foundation":     {header: "✻ Cài đặt"},
	"read_chapter":        {header: "✻ Đọc chương"},
	"check_consistency":   {header: "✻ Kiểm tra tính nhất quán"},
	"novel_context":       {header: "✻ Ngữ cảnh truy vấn"},
}

type toolDisplay struct {
	header   string
	nakedKey string
}

// jsonFieldExtractor là trình mã thông báo JSON phát trực tuyến. Điều khiển trạng thái máy theo từng byte, công cụ LLM
// args được chuyển đổi thành văn bản có thể đọc được. Phiên bản tương tự chỉ phục vụ một lệnh gọi công cụ và Done()=true sau khi đóng vùng chứa cấp cao nhất.
type jsonFieldExtractor struct {
	cfg toolDisplay

	state pState
	stack []byte // Ngăn xếp vùng chứa: 'O' obj / 'A' mảng

	keyBuf strings.Builder

	escape bool
	uHex   []byte

	started bool // Liệu có bất kỳ ký tự nào đã được phát ra hay không (được sử dụng để ngắt dòng giữa tiêu đề và khóa đầu tiên)

	done bool
}

type pState int

const (
	psRoot         pState = iota
	psBeforeKey           // Trong obj: đợi khóa tiếp theo hoặc }
	psInKey               // Trong obj: khóa phân tích
	psAfterKey            // Trong obj: chờ:
	psBeforeValue         // Đợi ký tự bắt đầu giá trị
	psStringStream        // giá trị chuỗi, phát trực tuyến ký tự đã nấu chín
	psStringSkip          // giá trị chuỗi, bỏ qua (các trường không phải mục tiêu ở chế độ phát trực tuyến trần trụi)
	psNumberStream        // kỹ thuật số, phát trực tuyến
	psNumberSkip          // số, bỏ qua
	psPrimStream          // đúng/sai/null, phát trực tuyến
	psPrimSkip            // đúng/sai/null, bỏ qua
	psDone                // Thùng trên cùng được đóng lại
)

func newToolExtractor(tool string) *jsonFieldExtractor {
	cfg, ok := toolDisplays[tool]
	if !ok {
		return nil
	}
	return &jsonFieldExtractor{cfg: cfg}
}

func (e *jsonFieldExtractor) Done() bool { return e.done }

func (e *jsonFieldExtractor) Feed(chunk string) string {
	if e.done || chunk == "" {
		return ""
	}
	var out strings.Builder
	for i := 0; i < len(chunk); i++ {
		e.step(chunk[i], &out)
		if e.done {
			break
		}
	}
	return out.String()
}

// ──Ngăn xếp/thụt vùng chứa──

func (e *jsonFieldExtractor) push(kind byte) {
	e.stack = append(e.stack, kind)
}

func (e *jsonFieldExtractor) pop() {
	if len(e.stack) == 0 {
		return
	}
	e.stack = e.stack[:len(e.stack)-1]
}

func (e *jsonFieldExtractor) parent() byte {
	if len(e.stack) == 0 {
		return 0
	}
	return e.stack[len(e.stack)-1]
}

// writeIndent ghi thụt lề hiện tại. Độ sâu = số mức lồng nhau = len(stack)-1 (vùng chứa gốc không được thụt lề).
func (e *jsonFieldExtractor) writeIndent(out *strings.Builder) {
	depth := len(e.stack) - 1
	for range depth {
		out.WriteString("  ")
	}
}

// ── Máy trạng thái ──

func (e *jsonFieldExtractor) step(c byte, out *strings.Builder) {
	switch e.state {
	case psRoot:
		switch c {
		case '{':
			e.push('O')
			e.state = psBeforeKey
		case '[':
			// Điều đó không thực sự xảy ra (các đối số của công cụ luôn là obj); dung nạp: khi root arr
			e.push('A')
			e.state = psBeforeValue
		}
	case psBeforeKey:
		switch c {
		case '"':
			e.keyBuf.Reset()
			e.escape = false
			e.state = psInKey
		case '}':
			e.closeContainer(out)
		case ' ', '\t', '\n', '\r', ',':
		}
	case psInKey:
		if e.escape {
			e.keyBuf.WriteByte(c)
			e.escape = false
			return
		}
		if c == '\\' {
			e.escape = true
			return
		}
		if c == '"' {
			e.emitKeyLine(out, e.keyBuf.String())
			e.state = psAfterKey
			return
		}
		e.keyBuf.WriteByte(c)
	case psAfterKey:
		if c == ':' {
			e.state = psBeforeValue
		}
	case psBeforeValue:
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' {
			return
		}
		switch c {
		case '"':
			e.beginString(out)
		case '{':
			e.beginNested('O', out)
		case '[':
			e.beginNested('A', out)
		case ']', '}':
			e.closeContainer(out)
		case 't', 'f', 'n':
			e.beginPrim(c, out)
		default:
			if c == '-' || (c >= '0' && c <= '9') {
				e.beginNumber(c, out)
			}
		}
	case psStringStream:
		e.handleStringByte(c, out, false)
	case psStringSkip:
		e.handleStringByte(c, out, true)
	case psNumberStream:
		if isNumberByte(c) {
			out.WriteByte(c)
			return
		}
		e.afterValueChar(c, out)
	case psNumberSkip:
		if isNumberByte(c) {
			return
		}
		e.afterValueChar(c, out)
	case psPrimStream:
		if c >= 'a' && c <= 'z' {
			out.WriteByte(c)
			return
		}
		e.afterValueChar(c, out)
	case psPrimSkip:
		if c >= 'a' && c <= 'z' {
			return
		}
		e.afterValueChar(c, out)
	case psDone:
	}
}

// ── Hiển thị dòng ──

// phátKeyLine được gọi khi khóa trong obj được phân tích cú pháp và tiền tố "<lf><indent>key:" được viết.
// Ở chế độ luồng trần, tiền tố khóa không được ghi (khóa được ghi trong keyBuf để BeginString đánh giá).
func (e *jsonFieldExtractor) emitKeyLine(out *strings.Builder, key string) {
	if e.cfg.nakedKey != "" {
		return
	}
	if !e.started {
		if e.cfg.header != "" {
			out.WriteString(e.cfg.header)
			out.WriteByte('\n')
		}
		e.started = true
	} else {
		out.WriteByte('\n')
	}
	e.writeIndent(out)
	out.WriteString(key)
	out.WriteByte(':')
}

// emitterArrayItem được gọi ở đầu mỗi phần tử trong mảng, viết "<lf><indent>-". nguyên thủy
// Các phần tử được theo sau bởi dấu cách trước khi phát ra giá trị; các phần tử cấu trúc được bao bọc một cách tự nhiên bằng cách lồng nhau tiếp theo.
func (e *jsonFieldExtractor) emitArrayItem(out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		return
	}
	if !e.started {
		if e.cfg.header != "" {
			out.WriteString(e.cfg.header)
			out.WriteByte('\n')
		}
		e.started = true
	} else {
		out.WriteByte('\n')
	}
	e.writeIndent(out)
	out.WriteByte('-')
}

// ── giá trị bắt đầu ──

func (e *jsonFieldExtractor) beginString(out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		// Luồng trần trụi: Chỉ xuất ra giá trị chuỗi của khóa đích trong obj cấp cao nhất
		if e.cfg.nakedKey == e.keyBuf.String() && len(e.stack) == 1 && e.stack[0] == 'O' {
			e.state = psStringStream
		} else {
			e.state = psStringSkip
		}
		e.escape = false
		e.uHex = nil
		return
	}
	// Chung: Trường obj ngay sau "key: " ("key:" đã được phát ra và sau đó các khoảng trắng được điền vào); phần tử mảng ngay sau "-"
	if e.parent() == 'A' {
		e.emitArrayItem(out)
		out.WriteByte(' ')
	} else {
		out.WriteByte(' ')
	}
	e.state = psStringStream
	e.escape = false
	e.uHex = nil
}

func (e *jsonFieldExtractor) beginNumber(first byte, out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		e.state = psNumberSkip
		return
	}
	if e.parent() == 'A' {
		e.emitArrayItem(out)
		out.WriteByte(' ')
	} else {
		out.WriteByte(' ')
	}
	out.WriteByte(first)
	e.state = psNumberStream
}

func (e *jsonFieldExtractor) beginPrim(first byte, out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		e.state = psPrimSkip
		return
	}
	if e.parent() == 'A' {
		e.emitArrayItem(out)
		out.WriteByte(' ')
	} else {
		out.WriteByte(' ')
	}
	out.WriteByte(first)
	e.state = psPrimStream
}

func (e *jsonFieldExtractor) beginNested(kind byte, out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		// Chế độ luồng trần không mở rộng việc lồng nhau; sử dụng độ sâu ngăn xếp để theo dõi kết quả khớp } / ]
		e.push(kind)
		if kind == 'O' {
			e.state = psBeforeKey
		} else {
			e.state = psBeforeValue
		}
		return
	}
	// Chế độ chung: Khi phần tử mảng là cấu trúc lồng nhau, trước tiên hãy phát ra "<indent>-" của một dòng
	// (Không có khoảng trắng sau dấu :// của khóa obj, cho phép khóa con lồng nhau bọc tự nhiên sang dòng tiếp theo)
	if e.parent() == 'A' {
		e.emitArrayItem(out)
	}
	e.push(kind)
	if kind == 'O' {
		e.state = psBeforeKey
	} else {
		e.state = psBeforeValue
	}
}

// closeContainer xử lý } hoặc ].
func (e *jsonFieldExtractor) closeContainer(out *strings.Builder) {
	e.pop()
	if len(e.stack) == 0 {
		// Các đối số trống (ví dụ: tiểu thuyết_context không truyền tham số) là một sự đảm bảo: emitKeyLine không có cơ hội xuất tiêu đề.
		// Hãy sửa lại ở đây để tránh rơi vào tình trạng “không tiêu đề cũng không nội dung”.
		if !e.started && e.cfg.nakedKey == "" && e.cfg.header != "" {
			out.WriteString(e.cfg.header)
			out.WriteByte('\n')
			e.started = true
		}
		// Một dòng mới ở cuối sẽ tạo ra một ranh giới rõ ràng giữa bảng điều khiển và phần đầu ra tiếp theo.
		if e.started {
			out.WriteByte('\n')
		}
		e.state = psDone
		e.done = true
		return
	}
	if e.parent() == 'O' {
		e.state = psBeforeKey
	} else {
		e.state = psBeforeValue
	}
}

// ── phát trực tiếp chuỗi ──

func (e *jsonFieldExtractor) handleStringByte(c byte, out *strings.Builder, skipping bool) {
	if e.uHex != nil {
		e.uHex = append(e.uHex, c)
		if len(e.uHex) == 4 {
			if r, ok := parseHex4(e.uHex); ok && !skipping {
				var buf [4]byte
				n := utf8.EncodeRune(buf[:], r)
				out.Write(buf[:n])
			}
			e.uHex = nil
		}
		return
	}
	if e.escape {
		e.escape = false
		if !skipping {
			writeEscapedByte(out, c)
		}
		if c == 'u' {
			e.uHex = make([]byte, 0, 4)
		}
		return
	}
	if c == '\\' {
		e.escape = true
		return
	}
	if c == '"' {
		e.afterValueDone()
		return
	}
	if !skipping {
		out.WriteByte(c)
	}
}

func writeEscapedByte(out *strings.Builder, c byte) {
	switch c {
	case 'n':
		out.WriteByte('\n')
	case 't':
		out.WriteByte('\t')
	case 'r':
		out.WriteByte('\r')
	case '"':
		out.WriteByte('"')
	case '\\':
		out.WriteByte('\\')
	case '/':
		out.WriteByte('/')
	case 'b', 'f':
		// Backspace / nguồn cấp dữ liệu biểu mẫu: bỏ qua
	case 'u':
		// Bộ đệm uHex được tạo bởi người gọi; không có đầu ra ở đây
	default:
		out.WriteByte('\\')
		out.WriteByte(c)
	}
}

// ── Đóng ──

// Chuỗi afterValueDone được đóng lại (đọc `"` ở cuối) và sau đó được chuyển sang trạng thái tiếp theo.
func (e *jsonFieldExtractor) afterValueDone() {
	e.escape = false
	e.uHex = nil
	if len(e.stack) == 0 {
		e.state = psDone
		e.done = true
		return
	}
	if e.parent() == 'O' {
		e.state = psBeforeKey
	} else {
		e.state = psBeforeValue
	}
}

// Khi "ký tự kết thúc" của số afterValueChar/nguyên thủy đã được đọc, trạng thái tiếp theo được xác định theo ký tự.
// Ký tự này có thể là , / } / ] / trống, được chức năng này chuyển tiếp và phân phối.
func (e *jsonFieldExtractor) afterValueChar(c byte, out *strings.Builder) {
	switch c {
	case '}', ']':
		e.closeContainer(out)
	case ',', ' ', '\t', '\n', '\r':
		if len(e.stack) == 0 {
			e.state = psDone
			e.done = true
			return
		}
		if e.parent() == 'O' {
			e.state = psBeforeKey
		} else {
			e.state = psBeforeValue
		}
	}
}

// ── Công cụ ──

func isNumberByte(c byte) bool {
	switch c {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'-', '+', '.', 'e', 'E':
		return true
	}
	return false
}

func parseHex4(b []byte) (rune, bool) {
	var r rune
	for _, d := range b {
		var v rune
		switch {
		case d >= '0' && d <= '9':
			v = rune(d - '0')
		case d >= 'a' && d <= 'f':
			v = rune(d-'a') + 10
		case d >= 'A' && d <= 'F':
			v = rune(d-'A') + 10
		default:
			return 0, false
		}
		r = r*16 + v
	}
	return r, true
}
