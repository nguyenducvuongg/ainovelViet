package flow

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/voocel/agentcore"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// Bộ điều phối tính toán các tuyến đường và đưa ra các hướng dẫn Máy chủ tại ranh giới công cụ đồng bộ hóa được tác nhân phụ trả về.
type Dispatcher struct {
	coordinator *agentcore.Agent
	store       *storepkg.Store

	enabled atomic.Bool // Việc gửi có được điều khiển bởi Máy chủ hay không (nên tắt trước khi khởi động xong)

	// Theo dõi lặp lại: Ghi nhớ Tác nhân+Nhiệm vụ cuối cùng được gửi đi và số lần gửi liên tiếp.
	// Lệnh tương tự được tính toán lặp đi lặp lại (trạng thái không được nâng cao sau khi tác nhân phụ quay trở lại và kết quả tính toán lại Tuyến đường vẫn không thay đổi) và không bị nuốt trong im lặng.
	// Thay vào đó, thực tế được truyền lại với số lần - "kết quả định tuyến giống nhau trong N lần liên tiếp" là thực tế mà chỉ Máy chủ mới có thể quan sát được;
	// Nếu im lặng, Điều phối viên sẽ rơi vào tình trạng "Cấm quyết định bước tiếp theo" (cođiều phối viên.md) và
	// Mâu thuẫn kép của "StopGuard", chơi miễn phí là vòng lặp vô hạn số 24 của người làm nghề tự do.
	// Quyền ra quyết định vẫn thuộc về LLM: việc gửi lại tin nhắn chỉ đi kèm với sự cho phép về thông tin thực tế và xác minh mà không cần đặt ngưỡng hoặc bộ ngắt mạch (Kiến trúc §10.13).
	// Các tin nhắn khác nhau do số lần chúng được gửi và các hướng dẫn có cùng nghĩa đen sẽ không được đẩy nhiều lần vào chỉ đạoQ.
	lastMu   sync.Mutex
	lastSent *Instruction
	repeats  int

	// onRepeat là một lệnh gọi lại đo từ xa thuần túy (được sử dụng cho các cảnh báo không được giám sát), trong cùng một lệnh lặp lạiNotifyAt
	// Nó được kích hoạt một lần khi nó được phát hành; nó không ảnh hưởng đến việc phân phối ngược lại và logic phân phối không biết đến sự tồn tại của nó.
	onRepeat func(agent, task string, n int)
}

// lặp lạiNotifyAt không được mã hóa cứng vào cấu hình: nó không phải là ngưỡng luồng điều khiển (nó không kích hoạt bất kỳ hành động nào, nó chỉ "gọi mọi người"),
// Không có lợi gì trong việc điều chỉnh nó; thay vào đó, cấu hình ngụ ý rằng sự khác biệt về hành vi có thể được điều chỉnh.
const repeatNotifyAt = 3

// NewDispatcher tạo một Bộ điều phối.
func NewDispatcher(coordinator *agentcore.Agent, store *storepkg.Store) *Dispatcher {
	d := &Dispatcher{coordinator: coordinator, store: store}
	return d
}

// Cho phép bật công văn tuyến đường; khi tắt, Dispatch không tạo ra hướng dẫn.
// Máy chủ được bật sau khi Bắt đầu/Tiếp tục hoàn thành lời nhắc đầu tiên để tránh xung đột với quá trình khởi động.
func (d *Dispatcher) Enable() { d.enabled.Store(true) }

// Điều phối ngay lập tức tính toán lộ trình và đưa ra hướng dẫn; nó có thể được Chủ nhà gọi vào những thời điểm đặc biệt (chẳng hạn như sau khi Tiếp tục).
func (d *Dispatcher) Dispatch() {
	if !d.enabled.Load() {
		return
	}
	state := LoadState(d.store)
	inst := Route(state)
	if inst == nil {
		return
	}
	n := d.trackRepeat(inst)
	// Nhiệm vụ của người viết: đánh dấu chương là đang tiến hành cùng lúc với việc gửi đi và phần phác thảo ở phía bên phải của giao diện người dùng sẽ ngay lập tức phản ánh "▸ đang tiến hành",
	// Không cần phải đợi plan_chapter thực sự thực thi (plan_chapter sẽ gọi lại StartChapter, idempotent).
	if inst.Agent == "writer" && inst.Chapter > 0 && d.store != nil {
		if err := d.store.Progress.ValidateChapterWork(inst.Chapter); err != nil {
			slog.Error("flow router refuses invalid writer dispatch", "module", "host.flow", "chapter", inst.Chapter, "err", err)
			return
		}
		if err := d.store.Progress.StartChapter(inst.Chapter); err != nil {
			slog.Warn("flow router pre-mark in-progress failed", "module", "host.flow", "chapter", inst.Chapter, "err", err)
		}
	}
	msg := formatDispatchMessage(inst, n)
	slog.Debug("flow router dispatch", "module", "host.flow", "agent", inst.Agent, "reason", inst.Reason, "repeat", n)
	d.coordinator.Steer(agentcore.UserMsg(msg))
}

// formatDispatchMessage tập hợp thông báo lệnh được cấp cho Điều phối viên.
// Khi n>1, nối thêm các sự kiện trùng lặp - thông báo "sự thật định tuyến không thay đổi kể từ lần gửi cuối cùng" và giải phóng quyền xác minh,
// Hãy để LLM quyết định thực hiện như bình thường hay lên lịch lại; không tạo bất kỳ nhánh bắt buộc nào ở lớp Máy chủ.
func formatDispatchMessage(inst *Instruction, n int) string {
	msg := FormatMessage(inst)
	if n > 1 {
		msg += fmt.Sprintf("\n (Lưu ý: Lệnh này được ban hành cho lần %d - các dữ kiện định tuyến không thay đổi kể từ lần gửi cuối cùng. Lần này, tiểu thuyết_context được phép gọi để kiểm tra các dữ kiện trước, sau đó đưa ra quyết định thực thi như bình thường hoặc gửi các tác nhân phụ khác.)", n)
	}
	return msg
}

// SetOnRepeat đăng ký cuộc gọi lại đo từ xa để có hướng dẫn lặp lại. Phải được gọi một lần trước khi bắt đầu phân phối.
func (d *Dispatcher) SetOnRepeat(cb func(agent, task string, n int)) {
	d.onRepeat = cb
}

// trackRepeat ghi lại số lần liên tiếp cùng một lệnh được đưa ra và trả về số hiện tại (1 = lệnh mới).
// Sử dụng đẳng thức Tác nhân+Nhiệm vụ (không tốt hơn Lý do, vì Lý do là văn bản phụ trợ để mọi người xem).
// Khi đạt đến số lần lặp lạiNotifyAt, onRepeat được kích hoạt bên ngoài khóa (khởi động lại sau số lần đặt lại thay đổi phím).
func (d *Dispatcher) trackRepeat(next *Instruction) int {
	d.lastMu.Lock()
	if d.lastSent != nil && d.lastSent.Agent == next.Agent && d.lastSent.Task == next.Task {
		d.repeats++
	} else {
		cp := *next
		d.lastSent = &cp
		d.repeats = 1
	}
	n := d.repeats
	d.lastMu.Unlock()

	if n == repeatNotifyAt && d.onRepeat != nil {
		d.onRepeat(next.Agent, next.Task, n)
	}
	return n
}

// ResetRepeat xóa theo dõi lặp lại. Tiếp tục / Được gọi bởi Máy chủ khi Bắt đầu mới,
// Đảm bảo rằng lệnh đầu tiên sau khi khôi phục hoặc tạo được đưa ra với ngữ nghĩa "lần đầu tiên".
func (d *Dispatcher) ResetRepeat() {
	d.lastMu.Lock()
	defer d.lastMu.Unlock()
	d.lastSent = nil
	d.repeats = 0
}
