// Gói thông báo cung cấp một kênh cảnh báo không được giám sát.
//
// Định vị hiến pháp (architecture.md §2.3): Hoạt động của lớp quan sát thuần túy - cảnh báo không bao giờ can thiệp vào luồng điều khiển
// (Không thử lại, không lên lịch lại, không tắt máy), chỉ "hét" các sự kiện hiện có trong TUI ra khỏi màn hình.
// Gửi được thực thi không đồng bộ, không bao giờ chặn Máy chủ và chỉ ghi lại nhật ký nếu thất bại.
package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Thông báo Tất cả các sự kiện của một báo động.
type Notification struct {
	Kind  string `json:"kind"`  // run_end / repeat / budget
	Level string `json:"level"` // info / warn / error
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Trình thông báo phân phối thông báo theo cấu hình. Giá trị 0 không có sẵn và phải được tạo bằng Mới; nil là an toàn (Gửi noop).
type Notifier struct {
	command string          // Thay thế kênh hệ thống khi nó không trống (chuyển hướng vào đây để đẩy điện thoại di động)
	events  map[string]bool // nil = tất cả các loại được phát hành
	timeout time.Duration
}

// Trình thông báo tạo mới. lệnh trống và sử dụng kênh hệ thống tích hợp (macOS osascript /
// Linux thông báo-gửi, nếu không tìm thấy lệnh, nó sẽ bị hạ cấp âm thầm xuống chỉ còn slog); chỉ loại được liệt kê mới được phát hành khi các sự kiện không trống.
func New(command string, events []string) *Notifier {
	n := &Notifier{command: strings.TrimSpace(command), timeout: 10 * time.Second}
	if len(events) > 0 {
		n.events = make(map[string]bool, len(events))
		for _, ev := range events {
			n.events[ev] = true
		}
	}
	return n
}

// Gửi gửi thông báo không đồng bộ. Việc lọc, thực thi và xử lý lỗi đều không ảnh hưởng đến người gọi.
func (n *Notifier) Send(nt Notification) {
	if !n.allows(nt.Kind) {
		return
	}
	go n.deliver(nt)
}

// cho phép trả về liệu loại đó có được phép hay không (không có Trình thông báo/bị chặn khi không được đưa vào các sự kiện).
func (n *Notifier) allows(kind string) bool {
	if n == nil {
		return false
	}
	return n.events == nil || n.events[kind]
}

// Deliver thực hiện gửi một cách đồng bộ (chạy trong goroutine; kiểm tra có thể gọi trực tiếp để đồng bộ hóa các xác nhận).
func (n *Notifier) deliver(nt Notification) {
	defer func() { recover() }()
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	var err error
	if n.command != "" {
		err = runCommand(ctx, n.command, nt)
	} else {
		err = runSystem(ctx, nt)
	}
	if err != nil {
		slog.Warn("Thông báo không gửi được", "module", "notify", "kind", nt.Kind, "err", err)
	}
}

// runCommand thực thi lệnh do người dùng định cấu hình: các trường được truyền vào thông qua các biến môi trường (một dòng cuộn tròn không có phần phụ thuộc và không có phần chèn
// rủi ro), JSON hoàn chỉnh sẽ được ghi vào stdin cùng lúc (các kịch bản phân phối phức tạp được tự phân tích cú pháp). Hết thời gian chờ bị giết bởi ctx.
func runCommand(ctx context.Context, command string, nt Notification) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(),
		"NOTIFY_KIND="+nt.Kind,
		"NOTIFY_LEVEL="+nt.Level,
		"NOTIFY_TITLE="+nt.Title,
		"NOTIFY_BODY="+nt.Body,
	)
	payload, _ := json.Marshal(nt)
	cmd.Stdin = strings.NewReader(string(payload))
	return cmd.Run()
}

// Thông báo trên màn hình tích hợp của runSystem: chỉ bao gồm tình huống "mọi người đang ở máy tính" và tự động giảm cấp nếu không tìm thấy lệnh nào.
func runSystem(ctx context.Context, nt Notification) error {
	switch runtime.GOOS {
	case "darwin":
		script := "display notification " + appleScriptString(nt.Body) + " with title " + appleScriptString(nt.Title)
		return exec.CommandContext(ctx, "osascript", "-e", script).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			slog.Info("Thông báo được hạ cấp xuống nhật ký (không gửi thông báo)", "module", "notify", "title", nt.Title, "body", nt.Body)
			return nil
		}
		return exec.CommandContext(ctx, "notify-send", nt.Title, nt.Body).Run()
	default:
		slog.Info("Thông báo bị hạ cấp xuống nhật ký (nền tảng không có kênh hệ thống)", "module", "notify", "title", nt.Title, "body", nt.Body)
		return nil
	}
}

// appleScriptString gói văn bản tùy ý thành một chuỗi ký tự AppleScript.
func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
