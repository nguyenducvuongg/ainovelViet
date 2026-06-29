package host

import (
	"fmt"
	"math"
	"sync/atomic"

	"github.com/voocel/agentcore"
	"github.com/nguyenducvuongg/ainovelViet/internal/bootstrap"
)

// Máy trạng thái ngân sách: lũy tiến đơn điệu, mỗi lần di chuyển sẽ kích hoạt chính xác một tác dụng phụ và không quay trở lại.
// Tăng ngân sách = ủy quyền lại người dùng = khởi động lại/phiên bản Máy chủ mới sau khi thay đổi cấu hình và không quay lại trạng thái trong trường hợp này.
const (
	budgetNormal      int32 = iota // Chưa đạt mực nước cảnh báo
	budgetWarned                   // Báo động được đưa ra, không vượt qua ranh giới
	budgetStopPending              // Dòng đã được vượt qua, chờ ranh giới đại lý phụ dừng lại.
	budgetStopped                  // Việc tắt máy đã được thực hiện
)

// BudgetSentinel giám sát chi phí tích lũy và thực thi chính sách ngân sách của người dùng (khối ngân sách cấu hình).
//
// Định vị hiến pháp (architecture.md §8.3/§10): Không đánh giá hành vi của mô hình - tắt máy ngoài luồng tương đương với người dùng
// Tại thời điểm hủy bỏ thủ công đó, Máy chủ chỉ cần thay mặt nó thực hiện một lệnh được ký trước. Nó ảnh hưởng đến luồng điều khiển, vì vậy
// Nó không phải là người quan sát và được định vị là thành phần chính sách Máy chủ ở cùng cấp độ với flow.Dispatcher; lớp Route/tool ​​​​không biết về nó.
//
// Thời gian ngừng hoạt động: Theo mặc định, nó nằm ở ranh giới tác nhân phụ (Máy chủ gọi HandleBoundary một cách đồng bộ), do đó không có chương nào trong quá trình hoạt động bị lãng phí;
// Khi hardStop=true, nó dừng ngay lập tức khi vượt qua vạch. Quá trình xử lý ranh giới xảy ra trước luồng. Bộ điều phối gửi bước tiếp theo và lớp Tuyến đường/Công cụ không nhận biết được ngân sách.
type BudgetSentinel struct {
	limit     float64
	warnRatio float64
	hardStop  bool

	costNow func() float64              // Chi phí tích lũy hiện tại (mức sử dụng. Trình bao bọc tổng số; cuống thử nghiệm có thể tiêm)
	abort   func(reason string)         // Đóng gói thời gian ngừng hoạt động của máy chủ (có sự kiện nguyên nhân)
	report  func(level, summary string) // Thoát cảnh báo (emitEvent + thông báo, được Host đưa vào)

	state atomic.Int32

	// Phát hiện điểm mù thanh toán: Đối với những mô hình có đăng ký là vô giá và nhà cung cấp không tự báo cáo chi phí, mỗi lần tăng hóa đơn là 0 USD.
	// Ngân sách hết hạn trong âm thầm. Xác định dựa trên "nhiều số tăng 0 liên tiếp" thay vì tổng==0 - số sau không thể bắt kịp giữa thời gian dài
	// /model chuyển sang khung cảnh của mô hình vô giá (tổng dừng ở giá trị lịch sử khác 0 nhưng không còn tăng nữa).
	// Mô hình miễn phí cũng thành công và lời nhắc "Ngân sách sẽ không kích hoạt" cũng đúng với mô hình đó.
	lastTotal   atomic.Uint64 // math.Float64bits (chi phí tích lũy của lần gọi lại cuối cùng)
	zeroStreak  atomic.Int32
	blindWarned atomic.Bool
}

// Báo động blindZeroStreak sau số lượng giao dịch kế toán không tăng liên tiếp. Trong mô hình định giá thông thường, mỗi mức tăng phải > 0
// (chi phí là chi phí thả nổi và được tích lũy mà không làm tròn). Giá trị 5 chỉ để tránh những trục trặc nghiêm trọng và không phải là ngưỡng chính sách có thể điều chỉnh.
const blindZeroStreak = 5

// NewBudgetSentinel Tạo một bản báo cáo ngân sách; trả về con số 0 nếu chính sách này không được bật (tất cả các phương thức đều không an toàn).
func NewBudgetSentinel(cfg bootstrap.BudgetConfig, costNow func() float64, abort func(reason string), report func(level, summary string)) *BudgetSentinel {
	if !cfg.Enabled() {
		return nil
	}
	return &BudgetSentinel{
		limit:     cfg.BookUSD,
		warnRatio: cfg.WarnRatio,
		hardStop:  cfg.HardStop,
		costNow:   costNow,
		abort:     abort,
		report:    report,
	}
}

// OnCost được gọi bởi UsageTracker (ngoài khóa) với chi phí tích lũy mới nhất sau mỗi lần hạch toán.
// Lệnh gọi lại có thể trải dài ở hai cấp độ (bình thường→cảnh báo→stopPending) và hai tác dụng phụ được kích hoạt một lần ở mỗi cấp độ.
func (s *BudgetSentinel) OnCost(total float64) {
	if s == nil {
		return
	}
	if prev := s.lastTotal.Swap(math.Float64bits(total)); total == math.Float64frombits(prev) {
		if s.zeroStreak.Add(1) >= blindZeroStreak && s.blindWarned.CompareAndSwap(false, true) {
			s.report("warn", fmt.Sprintf("Điểm mù ngân sách: Kế toán liên tục nhưng chi phí tích lũy dừng ở $%.2f và không tăng nữa (đăng ký mô hình hiện tại là vô giá và nhà cung cấp không tự báo cáo chi phí hoặc đó là mô hình miễn phí) - giới hạn ngân sách sẽ không được kích hoạt", total))
		}
	} else {
		s.zeroStreak.Store(0)
	}
	if total >= s.limit*s.warnRatio && s.state.CompareAndSwap(budgetNormal, budgetWarned) {
		s.report("warn", fmt.Sprintf("Cảnh báo ngân sách: đã chi $%.2f, đã đạt %.0f%% trong ngân sách $%.2f", total, s.limit, s.warnRatio*100))
	}
	if total >= s.limit && s.state.CompareAndSwap(budgetWarned, budgetStopPending) {
		if s.hardStop {
			s.report("error", fmt.Sprintf("Ngân sách đã cạn kiệt: $%.2f đã chi tiêu, $%.2f vượt ngân sách, ngừng hoạt động ngay lập tức", total, s.limit))
			s.stop(total)
			return
		}
		s.report("error", fmt.Sprintf("Ngân sách đã cạn: $%.2f đã chi, $%.2f vượt ngân sách, sẽ ngừng hoạt động sau khi nhiệm vụ của đại lý phụ hiện tại kết thúc", total, s.limit))
	}
}

// HandleEvent thực hiện việc tắt máy đang chờ xử lý ở ranh giới tác nhân phụ. Đăng ký phải đặt trước Người điều phối.
// IsError không bị bỏ qua - lỗi trả về cũng bị giới hạn và việc tắt máy không bị trì hoãn do lỗi proxy.
func (s *BudgetSentinel) HandleEvent(ev agentcore.Event) {
	if s == nil {
		return
	}
	if ev.Type != agentcore.EventToolExecEnd || ev.Tool != "subagent" {
		return
	}
	s.HandleBoundary()
}

func (s *BudgetSentinel) HandleBoundary() bool {
	if s == nil || s.state.Load() != budgetStopPending {
		return false
	}
	s.stop(s.costNow())
	return true
}

func (s *BudgetSentinel) stop(total float64) {
	if s.state.CompareAndSwap(budgetStopPending, budgetStopped) {
		s.abort(fmt.Sprintf("Thời gian ngừng hoạt động theo ngân sách: $%.2f đã được chi tiêu, $%.2f vượt quá ngân sách; hoạt động có thể được tiếp tục sau khi tăng ngân sách.book_usd", total, s.limit))
	}
}

// Từ chối bắt đầu kiểm tra trước: ngân sách đã bị vượt quá và trả về lỗi từ chối (cuộc gọi Bắt đầu/Tiếp tục/Tiếp tục đường dẫn khôi phục).
// Người dùng tăng ngân sách = ủy quyền lại và Từ chối được giải phóng tự nhiên theo cấu hình mới.
func (s *BudgetSentinel) Refuse() error {
	if s == nil {
		return nil
	}
	if cost := s.costNow(); cost >= s.limit {
		return fmt.Errorf("Cuốn sách này đã chi $%.2f, đạt đến giới hạn ngân sách là $%.2f; vui lòng tăng ngân sách cấu hình.book_usd và thử lại", cost, s.limit)
	}
	return nil
}

// Giới hạn trả về giới hạn trên của ngân sách (để hiển thị giao diện người dùng); trả về 0 nếu không được kích hoạt.
func (s *BudgetSentinel) Limit() float64 {
	if s == nil {
		return 0
	}
	return s.limit
}
