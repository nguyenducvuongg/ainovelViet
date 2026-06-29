package host

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/voocel/agentcore"
	corecontext "github.com/voocel/agentcore/context"
	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/agents"
	"github.com/voocel/ainovel-cli/internal/agents/ctxpack"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host/exp"
	"github.com/voocel/ainovel-cli/internal/host/flow"
	"github.com/voocel/ainovel-cli/internal/host/imp"
	"github.com/voocel/ainovel-cli/internal/host/sim"
	modelreg "github.com/voocel/ainovel-cli/internal/models"
	"github.com/voocel/ainovel-cli/internal/notify"
	"github.com/voocel/ainovel-cli/internal/rules"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/tools"
)

// Máy chủ là một lớp vỏ thời gian chạy mỏng.
// Trách nhiệm: Khởi động/Tiếp tục/Tiêm can thiệp/Chiếu sự kiện/Quản lý mô hình.
// Không có quyết định lập kế hoạch nào được thực hiện và không có sự tiếp tục nhàn rỗi nào được thực hiện.
type Host struct {
	cfg               bootstrap.Config
	bundle            assets.Bundle
	store             *storepkg.Store
	models            *bootstrap.ModelSet
	coordinator       *agentcore.Agent
	coordinatorCtxMgr *corecontext.ContextEngine // Liên kết SetContextWindow + SetReserveTokens khi chuyển đổi mô hình mặc định/điều phối viên
	thinkingApplier   agents.ApplyThinking       // /model Liên kết tác nhân trực tiếp (điều phối viên + tác nhân phụ) khi điều chỉnh cường độ tư duy
	askUser           *tools.AskUserTool
	writerRestore     *ctxpack.WriterRestorePack
	observer          *observer
	router            *flow.Dispatcher
	usage             *UsageTracker
	usageCancel       context.CancelFunc // Dừng autoSaveLoop và kích hoạt lần xả cuối cùng
	budget            *BudgetSentinel    // Chính sách ngân sách; không được kích hoạt dưới dạng nil (phương pháp nil an toàn)
	budgetDetach      func()
	notifier          *notify.Notifier // Báo động không giám sát; nil nếu không được kích hoạt (Gửi nil an toàn)

	events   chan Event
	streamCh chan string
	done     chan struct{}

	mu         sync.Mutex
	lifecycle  lifecycle
	cocreating bool // Chiếm giữ đồng sáng tạo giai đoạn: chặn sự can thiệp đồng thời của việc nhập/mô phỏng/tiếp tục trong cửa sổ bị tạm dừng
	closeOnce  sync.Once
}

type lifecycle string

const (
	lifecycleIdle      lifecycle = "idle"
	lifecycleRunning   lifecycle = "running"
	lifecyclePaused    lifecycle = "paused"
	lifecycleCompleted lifecycle = "completed"
)

// Tạo máy chủ mới.
func New(cfg bootstrap.Config, bundle assets.Bundle) (*Host, error) {
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		return nil, err
	}
	slog.Info("khởi động", "module", "boot", "provider", cfg.Provider, "model", cfg.ModelName, "output", cfg.OutputDir)

	// Bắt đầu quy trình nền để làm mới siêu dữ liệu mô hình (cửa sổ/giá) từ OpenRouter và lưu vào bộ nhớ đệm trên đĩa trong 24 giờ.
	modelreg.StartPricingRefresh(modelreg.DefaultRegistry(), bootstrap.DefaultConfigDir())

	store := storepkg.NewStore(cfg.OutputDir)
	if err := store.Init(); err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	models, err := bootstrap.NewModelSet(cfg)
	if err != nil {
		return nil, fmt.Errorf("create models: %w", err)
	}
	slog.Info("Mô hình đã sẵn sàng", "module", "boot", "summary", models.Summary())

	usage := NewUsageTracker(models, store)
	// Đọc meta/usage.json trước; sử dụng session/*.jsonl để chèn lấp một lần trong các trường hợp sau:
	//   - Tệp không tồn tại (nâng cấp lên phiên bản đầu tiên một cách kiên trì)
	//   - phiên bản lược đồ không khớp (định dạng cũ sẽ bị loại bỏ sau khi nâng cấp trong tương lai)
	//   - Tệp tồn tại nhưng bị hỏng / Lỗi IO (dữ liệu xấu không thể được phép thiết lập lại vĩnh viễn tích lũy về 0)
	// Sau khi chèn lấp, hãy Lưu ngay lập tức, củng cố kết quả và nhấn Tải trực tiếp vào lần tiếp theo bạn khởi động.
	loaded, loadErr := usage.LoadFromStore()
	if loadErr != nil {
		slog.Warn("không tải được mức sử dụng, sẽ cố gắng chèn lấp từ các phiên", "module", "usage", "err", loadErr)
	}
	if !loaded {
		if n, err := usage.ReplaySessions(cfg.OutputDir); err != nil {
			slog.Warn("phát lại sử dụng không thành công", "module", "usage", "err", err)
		} else if n > 0 {
			slog.Info("việc sử dụng được hoàn thành từ chèn lấp phiên", "module", "usage", "messages", n)
			if err := usage.SaveNow(); err != nil {
				slog.Warn("không lưu được mức sử dụng sau khi chèn lấp", "module", "usage", "err", err)
			}
		}
	}
	usageCtx, usageCancel := context.WithCancel(context.Background())
	usage.StartAutoSave(usageCtx)

	var router *flow.Dispatcher
	var budget *BudgetSentinel
	coordinator, askUser, restore, coordinatorCtxMgr, applyThinking := agents.BuildCoordinator(cfg, store, models, bundle, usage.Record, func(string) {
		if budget != nil && budget.HandleBoundary() {
			return
		}
		if router != nil {
			router.Dispatch()
		}
	})
	store.Signals.ClearStaleSignals()

	h := &Host{
		cfg:               cfg,
		bundle:            bundle,
		store:             store,
		models:            models,
		coordinator:       coordinator,
		coordinatorCtxMgr: coordinatorCtxMgr,
		thinkingApplier:   applyThinking,
		askUser:           askUser,
		writerRestore:     restore,
		usage:             usage,
		usageCancel:       usageCancel,
		events:            make(chan Event, 100),
		streamCh:          make(chan string, 256),
		done:              make(chan struct{}, 4),
		lifecycle:         lifecycleIdle,
	}
	h.observer = newObserver(coordinator, store, h.emitEvent, h.emitDelta, h.emitClear)
	if cfg.Notify.IsEnabled() {
		h.notifier = notify.New(cfg.Notify.Command, cfg.Notify.Events)
	}
	// Ngân sách trọng điểm đăng ký ngừng thực hiện sự kiện ranh giới của tác nhân phụ; Bộ điều phối được kích hoạt đồng bộ bởi chuỗi thực thi công cụ.
	// Không còn ưu tiên vòng gọi mô hình tiếp theo thông qua đăng ký sự kiện.
	if sentinel := NewBudgetSentinel(cfg.Budget,
		func() float64 { c, _, _, _, _ := usage.Totals(); return c },
		func(reason string) { h.abortWithEvent(reason, "error") },
		func(level, summary string) {
			h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: summary, Level: level})
			h.notifier.Send(notify.Notification{Kind: "budget", Level: level, Title: "ainovel: ngân sách", Body: summary})
		},
	); sentinel != nil {
		h.budget = sentinel
		budget = sentinel
		usage.SetOnCost(sentinel.OnCost)
		h.budgetDetach = coordinator.Subscribe(sentinel.HandleEvent)
		// Cảnh báo vùng mù thanh toán: Khi mô hình không báo cáo mức sử dụng, chi phí luôn bằng 0 và ngân sách không bao giờ được kích hoạt - cầu chì không được kết nối và phải gọi ai đó.
		usage.SetOnMissingUsage(func() {
			const blind = "Điểm mù ngân sách: Mô hình không trả về dữ liệu sử dụng, thống kê chi phí là 0 và giới hạn ngân sách sẽ không được kích hoạt (đối với các mô hình tùy chỉnh, vui lòng xác nhận giá đăng ký hoặc include_usage ngược dòng)"
			h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: blind, Level: "warn"})
			h.notifier.Send(notify.Notification{Kind: "budget", Level: "warn", Title: "ainovel: ngân sách", Body: blind})
		})
	}
	h.router = flow.NewDispatcher(coordinator, store)
	router = h.router
	// Cảnh báo lệnh trùng lặp: đo từ xa thuần túy, "mô hình có thể quay tại chỗ" khi treo máy. Thật đáng để gọi cho ai đó để xem.
	// Luồng sự kiện được phát theo cặp với thông báo—thông báo chỉ đơn giản là bản sao ngoài màn hình của sự kiện trên màn hình (Kiến trúc §2.3).
	h.router.SetOnRepeat(func(agent, task string, n int) {
		body := fmt.Sprintf("Lệnh tương tự đã được ban hành lần thứ %d (%s): %s", n, agent, task)
		h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Lặp lại lệnh: " + body, Level: "warn"})
		h.notifier.Send(notify.Notification{Kind: "repeat", Level: "warn", Title: "ainovel: hướng dẫn lặp lại", Body: body})
	})

	if err := store.RunMeta.Init(cfg.Style, cfg.Provider, cfg.ModelName); err != nil {
		slog.Error("Không thể khởi chạy thông tin meta chạy", "module", "boot", "err", err)
	}

	return h, nil
}

// ── Vòng đời ──

// Bắt đầu chế độ mới: khởi tạo tiến trình và bắt đầu vòng lặp dài của điều phối viên.
func (h *Host) Start(prompt string) error {
	return h.StartPrepared(BuildStartPrompt(prompt))
}

// StartPrepared bắt đầu soạn thảo bằng lời nhắc khởi động được lập trình.
func (h *Host) StartPrepared(promptText string) error {
	h.mu.Lock()
	if h.lifecycle == lifecycleRunning {
		h.mu.Unlock()
		return fmt.Errorf("already running")
	}
	if h.cocreating {
		h.mu.Unlock()
		return fmt.Errorf("Quá trình đồng sáng tạo theo giai đoạn đang diễn ra, vui lòng kết thúc quá trình đồng sáng tạo trước")
	}
	h.mu.Unlock()

	promptText = strings.TrimSpace(promptText)
	if promptText == "" {
		return fmt.Errorf("prompt is required")
	}
	if err := h.budget.Refuse(); err != nil {
		return err
	}
	if err := h.store.Checkpoints.Reset(); err != nil {
		return fmt.Errorf("reset checkpoints: %w", err)
	}
	if err := h.store.Progress.Init("", 0); err != nil {
		return fmt.Errorf("init progress: %w", err)
	}

	slog.Info("Bắt đầu tạo", "module", "host", "prompt_len", len(promptText))
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Bắt đầu tạo", Level: "info"})
	h.observer.setAborting(false)
	// Trước tiên hãy đặt lại theo dõi trùng lặp và bật định tuyến trước khi bắt đầu Nhắc để tránh vòng sự kiện đầu tiên đến trước khi bật
	h.router.ResetRepeat()
	h.router.Enable()
	if err := h.coordinator.Prompt(context.Background(), promptText); err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	// Chủ động gửi lệnh đầu tiên: nếu đã vào giai đoạn ghi (Giai đoạn=Viết) thì Host sẽ ra lệnh ngay;
	// Giai đoạn lập kế hoạch Lộ trình trả về con số 0, không có tác dụng phụ.
	h.router.Dispatch()

	h.mu.Lock()
	h.lifecycle = lifecycleRunning
	h.mu.Unlock()
	go h.waitDone()
	return nil
}

// Tiếp tục chế độ khôi phục: tạo lời nhắc tiếp tục từ điểm kiểm tra + tiến trình và bắt đầu.
func (h *Host) Resume() (string, error) {
	h.mu.Lock()
	if h.lifecycle == lifecycleRunning {
		h.mu.Unlock()
		return "", fmt.Errorf("already running")
	}
	if h.cocreating {
		h.mu.Unlock()
		return "", fmt.Errorf("Quá trình đồng sáng tạo theo giai đoạn đang diễn ra, vui lòng kết thúc quá trình đồng sáng tạo trước")
	}
	h.mu.Unlock()

	prompt, label, err := buildResumePrompt(h.store)
	if err != nil {
		return "", err
	}
	if label == "" {
		return "", nil // Chế độ mới, không phục hồi
	}
	if err := h.budget.Refuse(); err != nil {
		return "", err
	}

	slog.Info("tiếp tục tạo", "module", "host", "label", label)
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Tiếp tục tạo: " + label, Level: "info"})
	for _, w := range h.store.CheckConsistency() {
		slog.Warn("Báo động nhất quán", "module", "host", "detail", w)
		h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Cảnh báo tính nhất quán: " + w, Level: "warn"})
	}
	h.refreshWriterRestore()
	h.observer.setAborting(false)
	h.router.ResetRepeat()
	h.router.Enable()
	if err := h.coordinator.Prompt(context.Background(), prompt); err != nil {
		return "", fmt.Errorf("resume prompt: %w", err)
	}
	// Chủ động gửi lệnh đầu tiên một lần để tránh việc Điều phối viên chỉ trả lời văn bản cho lời nhắc khôi phục và StopGuard liên tục chặn lệnh đó.
	h.router.Dispatch()

	h.mu.Lock()
	h.lifecycle = lifecycleRunning
	h.mu.Unlock()
	go h.waitDone()
	return label, nil
}

// can thiệpMsg gói văn bản của người dùng vào một thông báo can thiệp được Điều phối viên nhận ra.
// Chỉ đạo và Tiếp tục có chung khung: lệnh người dùng của cả hai mục đều có tiền tố `[Can thiệp người dùng]`,
// Chỉ bằng cách này, việc phân loại can thiệp của điều phối viên.md mới có thể được kích hoạt ổn định. Nếu không, văn bản trống của Tiếp tục sẽ bỏ qua các quy tắc định tuyến,
// Điều phối viên đã mất neo danh mục và gửi nhầm các tác nhân phụ (điều này từng khiến "chương đã viết thay đổi" được gửi đến người viết và chạm vào người bảo vệ edit_chapter).
func interventionMsg(text string) agentcore.Message {
	return agentcore.UserMsg("[Sự can thiệp của người dùng] " + text)
}

// Tiếp tục Tiếp tục sử dụng lời nhắc được chỉ định. Được gọi khi người dùng nhập vào hộp nhập liệu sau khi tắt máy.
func (h *Host) Continue(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("text is required")
	}
	h.mu.Lock()
	if h.cocreating {
		h.mu.Unlock()
		return fmt.Errorf("Quá trình đồng sáng tạo theo giai đoạn đang diễn ra, vui lòng kết thúc quá trình đồng sáng tạo trước")
	}
	running := h.lifecycle == lifecycleRunning
	h.mu.Unlock()

	h.emitEvent(Event{Time: time.Now(), Category: "USER", Summary: "[Tiếp tục] " + text, Level: "info"})

	if running {
		h.coordinator.FollowUp(interventionMsg(text))
		return nil
	}
	// Sau khi tắt máy → Đưa vào và tự động khôi phục (quá trình khôi phục cũng phải tuân theo các ràng buộc về ngân sách ở giao diện người dùng)
	if err := h.budget.Refuse(); err != nil {
		return err
	}
	h.refreshWriterRestore()
	h.observer.setAborting(false)
	_, err := h.coordinator.Inject(interventionMsg(text))
	if err != nil {
		return fmt.Errorf("inject: %w", err)
	}
	h.mu.Lock()
	h.lifecycle = lifecycleRunning
	h.mu.Unlock()
	go h.waitDone()
	return nil
}

// Chỉ đạo gửi sự can thiệp của người dùng.
func (h *Host) Steer(text string) {
	h.mu.Lock()
	running := h.lifecycle == lifecycleRunning
	h.mu.Unlock()

	h.emitEvent(Event{Time: time.Now(), Category: "USER", Summary: "[Sự can thiệp của người dùng] " + text, Level: "info"})

	msg := interventionMsg(text)
	if running {
		if _, err := h.coordinator.Inject(msg); err != nil {
			slog.Error("tiêm chỉ đạo không thành công", "module", "host", "err", err)
		}
		return
	}
	// Tắt máy: duy trì cho đến lần khởi động tiếp theo + trạng thái hệ thống phản hồi ("Đã lưu" là lời nhắc hệ thống không phải là sự kiện NGƯỜI DÙNG)
	_ = h.store.RunMeta.SetPendingSteer(text)
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Sự can thiệp đã được lưu và sẽ có hiệu lực vào lần khởi động tiếp theo", Level: "info"})
}

// Hủy bỏ đình chỉ điều phối viên hiện tại.
func (h *Host) Abort() bool {
	return h.abortWithEvent("Người dùng tạm dừng quá trình tạo hiện tại theo cách thủ công", "warn")
}

// abortWithEvent Thực hiện hủy bỏ với sự kiện nguyên nhân được chỉ định. Tắt máy theo ngân sách và tạm dừng thủ công có cùng cơ chế tắt máy.
// Chỉ có bản sao sự kiện là khác (tắt theo ngân sách = Hủy bỏ hướng dẫn được người dùng ký trước, tương đương về mặt ngữ nghĩa với việc tạm dừng thủ công).
func (h *Host) abortWithEvent(summary, level string) bool {
	h.mu.Lock()
	running := h.lifecycle == lifecycleRunning
	if running {
		h.lifecycle = lifecyclePaused
	}
	h.mu.Unlock()
	if !running {
		return false
	}
	// Cài đặt phải ở trước điều phối viên.Abort: hủy truyền sẽ ngay lập tức kích hoạt luồng init/subagent
	// Các sự kiện thất bại, người quan sát sử dụng cờ này để xác định tiếng ồn có nguồn gốc từ việc hủy bỏ và ngăn chặn nó.
	h.observer.setAborting(true)
	h.coordinator.Abort()
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: summary, Level: level})
	return true
}

// Đóng chấm dứt điều phối viên và đóng kênh sự kiện.
//
// Ngữ nghĩa liên tục sử dụng: hủy autoSaveLoop trước (nó tự xóa trạng thái bẩn cuối cùng),
// Kết thúc bằng cách đồng bộ hóa lại SaveNow.Khoảng trống đã biết:AbortSilent Nếu vẫn còn trên chuyến bay LLM
// Khi được gọi lại, OnMessage → Record được kích hoạt sẽ cập nhật bộ nhớ nhưng sẽ không được duy trì. phần này
// Việc mất "vài trăm mã thông báo cuối cùng" sẽ được tự động bù đắp bằng phiên phát lại jsonl ở lần khởi động tiếp theo.
func (h *Host) Close() {
	h.observer.setAborting(true)
	h.coordinator.AbortSilent()
	if h.budgetDetach != nil {
		h.budgetDetach()
		h.budgetDetach = nil
	}
	if h.usageCancel != nil {
		h.usageCancel()
		h.usageCancel = nil
	}
	if err := h.usage.SaveNow(); err != nil {
		slog.Warn("Cách sử dụng Không thể đặt hàng trước khi thoát", "module", "usage", "err", err)
	}
	h.closeOnce.Do(func() {
		close(h.done)
		close(h.events)
		close(h.streamCh)
	})
}

// waitDone đợi điều phối viên dừng và xuất bản các sự kiện cuối cùng.
//
// Không có cuộc chạy tiếp theo nào được thực hiện. Kết thúc chạy = Máy chủ vào trạng thái cuối cùng:
//   - Giai đoạn=Hoàn thành → đánh dấu đã hoàn thành và gửi sự kiện "quá trình tạo đã hoàn thành"
//   - Khác → Đánh dấu là không hoạt động, gửi sự kiện "Điều phối viên dừng"
//
// Chỉ có hai cách để người dùng tiếp tục tạo: Continue thủ công (dừng tiêm) hoặc khởi động lại quá trình và sử dụng Resume.
// Xem tài liệu/architecture.md §13.3, §8.3.
func (h *Host) waitDone() {
	h.coordinator.WaitForIdle()
	h.observer.finalize()

	h.mu.Lock()
	progress, _ := h.store.Progress.Load()
	if progress != nil && progress.Phase == domain.PhaseComplete {
		h.lifecycle = lifecycleCompleted
		summary := fmt.Sprintf("Hoàn thành việc tạo: chương %d, từ %d", len(progress.CompletedChapters), progress.TotalWordCount)
		h.mu.Unlock()
		slog.Info(summary, "module", "host")
		h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: summary, Level: "success"})
		h.notifier.Send(notify.Notification{
			Kind: "run_end", Level: "info", Title: "ainovel: quá trình sáng tạo đã hoàn thành",
			Body: h.runEndBody(progress.NovelName, summary),
		})
	} else {
		wasRunning := h.lifecycle == lifecycleRunning
		if wasRunning {
			h.lifecycle = lifecycleIdle
		}
		completed := 0
		name := ""
		if progress != nil {
			completed = len(progress.CompletedChapters)
			name = progress.NovelName
		}
		h.mu.Unlock()
		if wasRunning {
			summary := fmt.Sprintf("Điều phối viên đã dừng (Chương %d đã hoàn thành)", completed)
			slog.Warn(summary, "module", "host")
			h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: summary, Level: "warn"})
			h.notifier.Send(notify.Notification{
				Kind: "run_end", Level: "warn", Title: "ainovel: điểm dừng sáng tạo",
				Body: h.runEndBody(name, summary),
			})
		}
	}

	select {
	case h.done <- struct{}{}:
	default:
	}
}

// runEndBody tập hợp văn bản thông báo run_end: tên sách + tóm tắt tiến độ + chi phí tích lũy.
func (h *Host) runEndBody(novelName, summary string) string {
	if name := strings.TrimSpace(novelName); name != "" {
		summary = "《" + name + "》" + summary
	}
	cost, _, _, _, _ := h.usage.Totals()
	if cost > 0 {
		summary += fmt.Sprintf(" · Chi phí $%.2f", cost)
	}
	return summary
}

// ── Kênh ──

// StreamClearSentinel được gửi qua streamingCh trong một dòng duy nhất để biểu thị "xóa vòng phát trực tuyến hiện tại".
// ClearCh độc lập không còn được sử dụng - rối loạn kênh đôi khiến tiêu đề ✻ thường rơi về cuối hiệp trước.
const StreamClearSentinel = "\x00\x00CLEAR\x00\x00"

func (h *Host) Events() <-chan Event        { return h.events }
func (h *Host) Stream() <-chan string       { return h.streamCh }
func (h *Host) Done() <-chan struct{}       { return h.done }
func (h *Host) Dir() string                 { return h.store.Dir() }
func (h *Host) AskUser() *tools.AskUserTool { return h.askUser }

// ──Phát ra sự kiện──

func (h *Host) emitEvent(ev Event) {
	defer func() { recover() }()
	// Mục khẩu hiệu duy nhất cho tất cả các sự kiện. các sự kiện Agentcore được người quan sát dịch và máy chủ tự phát
	// Tất cả các sự kiện HỆ THỐNG (Bắt đầu/Hủy/Tiếp tục...) đều được ghi vào đây để tránh việc hủy bỏ ESC và các sự kiện bên ngoài.
	// Việc chấm dứt không thể phân biệt được trên thui.log.
	if ev.Summary != "" || ev.Detail != "" {
		level := slog.LevelInfo
		switch ev.Level {
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
		// Nhật ký ghi lại Chi tiết đầy đủ (để khắc phục sự cố, không cắt bớt); chỉ khi Chi tiết trống, nó mới trở về Tóm tắt.
		msg := ev.Detail
		if msg == "" {
			msg = ev.Summary
		}
		attrs := []any{"module", "event", "category", ev.Category, "agent", ev.Agent}
		if ev.Kind != "" {
			attrs = append(attrs, "kind", ev.Kind)
		}
		slog.Log(context.Background(), level, msg, attrs...)
	}
	select {
	case h.events <- ev:
	default:
		select {
		case <-h.events:
		default:
		}
		select {
		case h.events <- ev:
		default:
		}
	}
}

func (h *Host) emitDelta(delta string) {
	defer func() { recover() }()
	select {
	case h.streamCh <- delta:
	default:
		select {
		case <-h.streamCh:
		default:
		}
		select {
		case h.streamCh <- delta:
		default:
		}
	}
}

func (h *Host) emitClear() {
	// Sử dụng "sentinel" thông qua streamingCh để đảm bảo phân phối có trật tự tới TUI trên cùng kênh với detectDelta.
	h.emitDelta(StreamClearSentinel)
}

// ── Ảnh chụp nhanh (tổng hợp trạng thái TUI) ──

func (h *Host) Snapshot() UISnapshot {
	h.mu.Lock()
	state := h.lifecycle
	provider, model, _ := h.models.CurrentSelection("default")
	h.mu.Unlock()

	// Phân tích cú pháp động cửa sổ ngữ cảnh của mô hình hiện tại và tự động phản ánh nó trong Ảnh chụp nhanh tiếp theo sau khi chuyển đổi /model
	modelWindow, _ := h.cfg.ResolveContextWindow(model)
	cost, tokIn, tokOut, cacheRead, cacheWrite := h.usage.Totals()
	saved := h.usage.SavedUSD()
	overallCapable := h.usage.OverallCacheCapable()
	recentRead, recentInput, recentSamples := h.usage.OverallRecent()
	perAgent := h.usage.PerAgent()
	cacheStats := make([]AgentCacheStat, 0, len(perAgent))
	for _, a := range perAgent {
		cacheStats = append(cacheStats, AgentCacheStat{
			Role:            a.Role,
			Input:           a.Input,
			Output:          a.Output,
			CacheRead:       a.CacheRead,
			CacheWrite:      a.CacheWrite,
			Cost:            a.Cost,
			Saved:           a.Saved,
			CacheCapable:    a.CacheCapable,
			RecentCacheRead: a.RecentCacheRead,
			RecentInput:     a.RecentInput,
			RecentSamples:   a.RecentSamples,
		})
	}
	perModel := h.usage.PerModel()
	modelStats := make([]AgentCacheStat, 0, len(perModel))
	for _, a := range perModel {
		modelStats = append(modelStats, AgentCacheStat{
			Model:        a.Model,
			Input:        a.Input,
			Output:       a.Output,
			CacheRead:    a.CacheRead,
			CacheWrite:   a.CacheWrite,
			Cost:         a.Cost,
			Saved:        a.Saved,
			CacheCapable: a.CacheCapable,
		})
	}

	snap := UISnapshot{
		Provider:               provider,
		ModelName:              model,
		ModelContextWindow:     modelWindow,
		Style:                  h.cfg.Style,
		RuntimeState:           string(state),
		IsRunning:              state == lifecycleRunning,
		TotalInputTokens:       tokIn,
		TotalOutputTokens:      tokOut,
		TotalCacheReadTokens:   cacheRead,
		TotalCacheWriteTokens:  cacheWrite,
		TotalCostUSD:           cost,
		TotalSavedUSD:          saved,
		BudgetLimitUSD:         h.budget.Limit(),
		OverallCacheCapable:    overallCapable,
		OverallRecentCacheRead: recentRead,
		OverallRecentInput:     recentInput,
		OverallRecentSamples:   recentSamples,
		CachePerAgent:          cacheStats,
		CachePerModel:          modelStats,
		MissingAssistantUsage:  h.usage.MissingAssistantUsage(),
	}

	progress, _ := h.store.Progress.Load()
	if progress != nil {
		snap.NovelName = strings.TrimSpace(progress.NovelName)
		snap.Phase = string(progress.Phase)
		snap.Flow = string(progress.Flow)
		snap.CurrentChapter = progress.CurrentChapter
		snap.TotalChapters = progress.TotalChapters
		snap.CompletedCount = len(progress.CompletedChapters)
		snap.TotalWordCount = progress.TotalWordCount
		snap.InProgressChapter = progress.InProgressChapter
		snap.PendingRewrites = progress.PendingRewrites
		snap.RewriteReason = progress.RewriteReason
		snap.Layered = progress.Layered
		if progress.CurrentVolume > 0 {
			snap.CurrentVolumeArc = fmt.Sprintf("Khối lượng %d·Cung %d", progress.CurrentVolume, progress.CurrentArc)
		}
	}
	if snap.NovelName == "" {
		if premise, _ := h.store.Outline.LoadPremise(); premise != "" {
			snap.NovelName = domain.ExtractNovelNameFromPremise(premise)
		}
	}
	if meta, _ := h.store.RunMeta.Load(); meta != nil {
		snap.PendingSteer = meta.PendingSteer
	}

	snap.Agents = h.observer.agentSnapshots()
	h.fillContextStatus(&snap)
	snap.StatusLabel = deriveStatusLabel(snap)

	// nhãn phục hồi
	if _, label, err := buildResumePrompt(h.store); err == nil && label != "" {
		snap.RecoveryLabel = label
	}

	h.fillDetails(&snap, progress)

	return snap
}

// fillContextStatus điền thông tin tình trạng ngữ cảnh của Điều phối viên.
func (h *Host) fillContextStatus(snap *UISnapshot) {
	if h.coordinator == nil {
		return
	}
	if usage := h.coordinator.BaselineContextUsage(); usage != nil {
		snap.ContextTokens = usage.Tokens
		snap.ContextWindow = usage.ContextWindow
		snap.ContextPercent = usage.Percent
	}
	if ctx := h.coordinator.ContextSnapshot(); ctx != nil {
		snap.ContextScope = ctx.Scope
		snap.ContextStrategy = ctx.LastStrategy
		snap.ContextActiveMessages = ctx.ActiveMessages
		snap.ContextSummaryCount = ctx.SummaryMessages
		snap.ContextCompactedCount = ctx.LastCompactedCount
		snap.ContextKeptCount = ctx.LastKeptCount
		if snap.ContextTokens == 0 {
			if ctx.BaselineUsage != nil {
				snap.ContextTokens = ctx.BaselineUsage.Tokens
				snap.ContextWindow = ctx.BaselineUsage.ContextWindow
				snap.ContextPercent = ctx.BaselineUsage.Percent
			} else if ctx.Usage != nil {
				snap.ContextTokens = ctx.Usage.Tokens
				snap.ContextWindow = ctx.Usage.ContextWindow
				snap.ContextPercent = ctx.Usage.Percent
			}
		}
	}
}

// fillDetails điền vào khu vực chi tiết: cài đặt, vai trò, cam kết/đánh giá/tóm tắt gần đây.
func (h *Host) fillDetails(snap *UISnapshot, progress *domain.Progress) {
	if premise, _ := h.store.Outline.LoadPremise(); premise != "" {
		snap.Premise = truncate(premise, 80)
	}
	if outline, _ := h.store.Outline.LoadOutline(); len(outline) > 0 {
		for _, e := range outline {
			snap.Outline = append(snap.Outline, OutlineSnapshot{
				Chapter: e.Chapter, Title: e.Title, CoreEvent: e.CoreEvent,
			})
		}
	}
	if progress != nil && progress.Layered {
		if compass, _ := h.store.Outline.LoadCompass(); compass != nil {
			snap.CompassDirection = compass.EndingDirection
			snap.CompassScale = compass.EstimatedScale
		}
		if volumes, _ := h.store.Outline.LoadLayeredOutline(); len(volumes) > 0 {
			for _, v := range volumes {
				if v.Index > progress.CurrentVolume {
					snap.NextVolumeTitle = v.Title
					break
				}
			}
		}
	}
	if chars, _ := h.store.Characters.Load(); len(chars) > 0 {
		for _, c := range chars {
			label := c.Name
			if c.Role != "" {
				label += "（" + c.Role + "）"
			}
			snap.Characters = append(snap.Characters, label)
		}
	}
	if ledger, _ := h.store.Cast.Load(); len(ledger) > 0 {
		snap.SupportingCount = len(ledger)
		recent, _ := h.store.Cast.RecentActive(5)
		for _, e := range recent {
			label := e.Name
			if e.BriefRole != "" {
				label += "（" + e.BriefRole + "）"
			}
			snap.RecentSupporting = append(snap.RecentSupporting, label)
		}
	}
	if progress != nil && len(progress.CompletedChapters) > 0 {
		lastCh := progress.CompletedChapters[len(progress.CompletedChapters)-1]
		wc := progress.ChapterWordCounts[lastCh]
		snap.LastCommitSummary = fmt.Sprintf("Chương %d %d từ", lastCh, wc)
	}
	currentCh := 1
	if progress != nil && len(progress.CompletedChapters) > 0 {
		currentCh = progress.CompletedChapters[len(progress.CompletedChapters)-1]
	}
	if review, err := h.store.World.LoadLastReview(currentCh); err == nil && review != nil {
		snap.LastReviewSummary = fmt.Sprintf("phán quyết=%s %d câu hỏi", review.Verdict, len(review.Issues))
		if len(review.AffectedChapters) > 0 {
			snap.LastReviewSummary += fmt.Sprintf(" Ảnh hưởng%v", review.AffectedChapters)
		}
	}
	if cp := h.store.Checkpoints.LatestGlobal(); cp != nil {
		snap.LastCheckpointName = fmt.Sprintf("%s.%s", cp.Scope, cp.Step)
	}
	if progress != nil {
		for i := len(progress.CompletedChapters) - 1; i >= 0 && len(snap.RecentSummaries) < 2; i-- {
			ch := progress.CompletedChapters[i]
			if summary, err := h.store.Summaries.LoadSummary(ch); err == nil && summary != nil {
				snap.RecentSummaries = append(snap.RecentSummaries,
					fmt.Sprintf("Chương %d: %s", ch, truncate(summary.Summary, 50)))
			}
		}
	}
}

func deriveStatusLabel(s UISnapshot) string {
	switch {
	case s.Phase == string(domain.PhaseComplete):
		return "COMPLETE"
	case s.Flow == string(domain.FlowReviewing):
		return "REVIEW"
	case s.Flow == string(domain.FlowRewriting) || s.Flow == string(domain.FlowPolishing):
		return "REWRITE"
	case s.RuntimeState == "running":
		return "RUNNING"
	default:
		return "READY"
	}
}

// ── Quản lý mô hình ──

func (h *Host) ConfiguredProviders() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	providers := make([]string, 0, len(h.cfg.Providers))
	for name := range h.cfg.Providers {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return providers
}

func (h *Host) ConfiguredModels(provider string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cfg.CandidateModels(provider)
}

func (h *Host) CurrentModelSelection(role string) (string, string, bool) {
	return h.models.CurrentSelection(role)
}

func (h *Host) SwitchModel(role, provider, model string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if provider == "" || model == "" {
		return fmt.Errorf("provider and model are required")
	}
	if err := h.models.Swap(role, provider, model); err != nil {
		return err
	}
	if role == "" || role == "default" {
		h.cfg.Provider = provider
		h.cfg.ModelName = model
	} else {
		if h.cfg.Roles == nil {
			h.cfg.Roles = make(map[string]bootstrap.RoleConfig)
		}
		rc := h.cfg.Roles[role]
		rc.Provider = provider
		rc.Model = model
		h.cfg.Roles[role] = rc
	}
	h.normalizeThinkingLocked(role)
	if path := bootstrap.DefaultConfigPath(); path != "" {
		if err := bootstrap.SaveConfig(path, h.cfg); err != nil {
			slog.Warn("Không lưu được cấu hình", "module", "host", "err", err)
		}
	}
	h.applyThinkingLocked(role)
	// Khi chuyển sang dòng chưa đăng ký gõ dòng cảnh báo nhắc người dùng bỏ ra 128k - bài viết dài dễ bị nén trước.
	logRole := role
	if logRole == "" {
		logRole = "default"
	}
	window, source := h.cfg.ResolveContextWindow(model)
	bootstrap.LogContextWindowChoice(logRole, model, window, source)

	// Khi chuyển sang mặc định/điều phối viên, cửa sổ công cụ điều phối và dự trữ được liên kết.
	// Người viết/kiến trúc sư/biên tập viên sử dụng ContextManagerFactory để tự động xây dựng lại theo mô hình mới mà không cần liên kết.
	// Không liên kết sẽ dẫn đến: 1M → 128k. Khi chuyển đổi, công cụ điều phối vẫn tính ngưỡng là 1M.
	// Nếu số tin nhắn tích lũy vượt quá 128k, API sẽ báo lỗi; khi 128k → 1M, ngưỡng được đặt ở mức 96k, gây lãng phí bối cảnh dài.
	//
	// Chìa khóa: Bạn phải sử dụng models. CurrentSelection("điều phối viên") để lấy mô hình mà "điều phối viên thực sự sử dụng"
	// Cửa sổ tính toán - thay vì sử dụng trực tiếp mô hình mục tiêu chuyển đổi. Khi người dùng được định cấu hình với mô hình riêng biệt của vai trò. điều phối viên,
	// Việc chuyển sang mặc định không ảnh hưởng đến mô hình điều phối viên thực tế; sẽ là sai lầm khi sử dụng SetContextWindow để chuyển đổi cửa sổ mục tiêu.
	// Điều chỉnh ngưỡng điều phối viên thành giá trị không phù hợp (ví dụ: khi chuyển sang mô hình 1M, hãy sử dụng điều phối viên mặc định là 200k
	// Ngưỡng công cụ được nâng lên 891k và ghi trên 200k sẽ trực tiếp làm nổ tung API).
	if h.coordinatorCtxMgr != nil && (role == "" || role == "default" || role == "coordinator") {
		_, coordinatorModel, _ := h.models.CurrentSelection("coordinator")
		coordinatorWindow, coordSource := h.cfg.ResolveContextWindow(coordinatorModel)
		h.coordinator.SetContextWindow(coordinatorWindow)
		h.coordinatorCtxMgr.SetContextWindow(coordinatorWindow)
		h.coordinatorCtxMgr.SetReserveTokens(bootstrap.CompactReserveTokens(coordinatorWindow))
		// Khi mô hình thực tế của điều phối viên khác với mục tiêu chuyển đổi (người dùng chuyển sang mặc định nhưng điều phối viên có vai trò độc quyền),
		// LogContextWindowChoice ở trên là cửa sổ mặc định, không nhất quán với giá trị hiệu quả thực tế; thêm một dòng.
		if coordinatorModel != model {
			bootstrap.LogContextWindowChoice("coordinator", coordinatorModel, coordinatorWindow, coordSource)
		}
	}

	h.emitEvent(Event{
		Time:     time.Now(),
		Category: "SYSTEM",
		Summary:  fmt.Sprintf("Chuyển đổi mô hình: %s → %s/%s", role, provider, model),
		Level:    "info",
	})
	return nil
}

// các vai trò tư duy cụ thể là những vai trò cụ thể mà cường độ tư duy có thể được áp dụng (phù hợp với các tác nhân. Lộ trình tư duy áp dụng).
// Khi điều chỉnh mặc định, hãy áp dụng lại ResolveThinking từng cái một theo từng vai trò.
var concreteThinkingRoles = []string{"coordinator", "architect", "writer", "editor"}

// CurrentThinking trả về chuỗi ban đầu về cường độ suy nghĩ hiện tại của một nhân vật (để bảng /model đồng bộ hóa giá trị hiện tại).
func (h *Host) CurrentThinking(role string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cfg.ResolveThinking(strings.ToLower(strings.TrimSpace(role)))
}

func (h *Host) AvailableThinking(role string) []agentcore.ThinkingLevel {
	h.mu.Lock()
	model := h.models.ForRole(strings.ToLower(strings.TrimSpace(role)))
	h.mu.Unlock()
	return agents.AvailableThinkingForModel(model)
}

func (h *Host) normalizeThinkingLocked(role string) agentcore.ThinkingLevel {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" || role == "default" {
		parsed, _ := agents.ParseThinkingLevel(h.cfg.Thinking)
		for _, r := range concreteThinkingRoles {
			resolved, ok := agents.ResolveThinkingForModel(h.models.ForRole(r), parsed)
			if !ok || resolved != parsed {
				h.cfg.Thinking = string(resolved)
				return resolved
			}
		}
		h.cfg.Thinking = string(parsed)
		return parsed
	}

	_, hasRoleThinking := h.cfg.Roles[role]
	hasRoleThinking = hasRoleThinking && h.cfg.Roles[role].Thinking != ""
	parsed, _ := agents.ParseThinkingLevel(h.cfg.ResolveThinking(role))
	resolved, _ := agents.ResolveThinkingForModel(h.models.ForRole(role), parsed)
	if !hasRoleThinking {
		if resolved != parsed {
			h.cfg.Thinking = string(resolved)
		}
		return resolved
	}
	if h.cfg.Roles == nil {
		h.cfg.Roles = make(map[string]bootstrap.RoleConfig)
	}
	rc := h.cfg.Roles[role]
	rc.Thinking = string(resolved)
	h.cfg.Roles[role] = rc
	return resolved
}

func (h *Host) applyThinkingLocked(role string) {
	if h.thinkingApplier == nil {
		return
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" || role == "default" {
		for _, r := range concreteThinkingRoles {
			lv, _ := agents.ParseThinkingLevel(h.cfg.ResolveThinking(r))
			h.thinkingApplier(r, lv)
		}
		return
	}
	lv, _ := agents.ParseThinkingLevel(h.cfg.ResolveThinking(role))
	h.thinkingApplier(role, lv)
}

// SetRoleThinking đặt cường độ suy nghĩ của một vai trò nhất định (hoặc mặc định): xác minh → kiên trì → liên kết tác nhân trực tiếp → sự kiện.
// Phản chiếu cấu trúc của SwitchModel; trực giao với việc lựa chọn mô hình và có thể được điều chỉnh độc lập. cấp độ trống = không ghi đè (được kế thừa).
func (h *Host) SetRoleThinking(role, level string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	parsed, err := agents.ParseThinkingLevel(level)
	if err != nil {
		return err
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" || role == "default" {
		for _, r := range concreteThinkingRoles {
			if resolved, ok := agents.ResolveThinkingForModel(h.models.ForRole(r), parsed); !ok || resolved != parsed {
				parsed = resolved
				break
			}
		}
	} else {
		parsed, _ = agents.ResolveThinkingForModel(h.models.ForRole(role), parsed)
	}
	// Kiên trì: Viết Vai trò[vai trò]. Suy nghĩ về các vai trò cụ thể và viết Suy nghĩ cấp cao nhất cho mặc định/"".
	if role == "" || role == "default" {
		h.cfg.Thinking = string(parsed)
	} else {
		if h.cfg.Roles == nil {
			h.cfg.Roles = make(map[string]bootstrap.RoleConfig)
		}
		rc := h.cfg.Roles[role]
		rc.Thinking = string(parsed)
		h.cfg.Roles[role] = rc
	}
	if path := bootstrap.DefaultConfigPath(); path != "" {
		if err := bootstrap.SaveConfig(path, h.cfg); err != nil {
			slog.Warn("Không lưu được cấu hình", "module", "host", "err", err)
		}
	}

	// Liên kết trực tiếp: áp dụng trực tiếp vào các vai trò cụ thể; mặc định đi qua từng vai trò cụ thể và áp dụng lại theo ResolveThinking
	// (Những thứ bị ghi đè theo cấp độ ký tự sẽ giữ nguyên, còn những thứ chưa bị ghi đè sẽ lấy mặc định mới).
	h.applyThinkingLocked(role)

	logRole := role
	if logRole == "" {
		logRole = "default"
	}
	shown := string(parsed)
	if shown == "" {
		shown = "Mặc định (kế thừa)"
	}
	h.emitEvent(Event{
		Time:     time.Now(),
		Category: "SYSTEM",
		Summary:  fmt.Sprintf("Cường độ suy nghĩ đã được chuyển đổi: %s → %s", logRole, shown),
		Level:    "info",
	})
	return nil
}

// ──Phát lại sự kiện──

func (h *Host) ReplayQueue(afterSeq int64) ([]domain.RuntimeQueueItem, error) {
	if h.store == nil || h.store.Runtime == nil {
		return nil, nil
	}
	return h.store.Runtime.LoadQueueAfter(afterSeq)
}

// ──Đồng sáng tạo──

// Đồng sáng tạo khởi đầu nguội CoCreateStream: làm rõ các yêu cầu từ đầu và đưa ra hướng dẫn tạo cho toàn bộ cuốn sách.
func (h *Host) CoCreateStream(ctx context.Context, history []CoCreateMessage, onProgress func(kind, text string)) (CoCreateReply, error) {
	return coCreateStream(ctx, h.models, h.store.Sessions, coCreateSystemPrompt, history, onProgress)
}

// StageCoCreateStream Đồng sáng tạo giai đoạn: Lập kế hoạch cho các hướng tiếp theo dựa trên những gì đã được viết.
// Lời nhắc của hệ thống = lời nhắc giai đoạn + tóm tắt trạng thái câu chuyện hiện tại, cho trợ lý biết "đã viết gì".
func (h *Host) StageCoCreateStream(ctx context.Context, history []CoCreateMessage, onProgress func(kind, text string)) (CoCreateReply, error) {
	return coCreateStream(ctx, h.models, h.store.Sessions, stageSystemPrompt(h.store), history, onProgress)
}

// stagePlanPrefix đóng gói "bản tóm tắt hướng dẫn tiếp theo" của đầu ra đồng sáng tạo vào một biện pháp can thiệp lập kế hoạch giai đoạn và gửi cho Điều phối viên để quyết định.
// Chỉ đăng thông tin [quy hoạch giai đoạn] + tuyên bố trung lập, không ghi "cách thực hiện" - lộ trình cụ thể (la bàn/kiến trúc sư/
// save_directive) được chuyển giao cho tiêu chí "lập kế hoạch giai đoạn" của điều phối viên.md để tránh hình thành nguồn sự thật thứ hai một cách nhanh chóng.
// Nó không chặn yêu cầu đối với các danh mục kiểu sử dụng chỉ thị (tuân theo "quyết định danh mục thuộc về LLM"). Tiếp tục và sau đó chồng thêm tiền tố [Can thiệp người dùng].
const stagePlanPrefix = "[Lập kế hoạch giai đoạn] Tôi đã tạm dừng quá trình tạo và làm việc với trợ lý đồng sáng tạo để sắp xếp các hướng tiếp theo sau. Vui lòng quyết định cách triển khai nó theo phân loại can thiệp của bạn và sau đó tiếp tục tạo. Hướng đi tiếp theo như sau: \n\n"

// PauseForCoCreate bước vào giai đoạn đồng sáng tạo: đặt dấu hiệu chiếm dụng đồng sáng tạo và tạm dừng điều phối viên trong khi hoạt động.
// Trả về sai có nghĩa là không thể nhập được (sách đã hoàn thành hoặc đang được đồng tạo) và người gọi có thể bỏ qua nó.
// Dấu nghề nghiệp chặn sự can thiệp đồng thời của thao tác nhập/mô phỏng/bắt đầu/tiếp tục/tiếp tục trong cửa sổ đồng sáng tạo——
// Sau khi vòng đời=tạm dừng trong quá trình hoạt động, mutex ==đang chạy hiện tại trở nên không hợp lệ và dấu này được sử dụng để lấp đầy khoảng trống;
// Đã dừng (không hoạt động/tạm dừng) cũng được phép vào. Sau khi lập kế hoạch, tiếp tục chạy.
func (h *Host) PauseForCoCreate() bool {
	h.mu.Lock()
	if h.cocreating || h.lifecycle == lifecycleCompleted {
		h.mu.Unlock()
		return false
	}
	h.cocreating = true
	running := h.lifecycle == lifecycleRunning
	h.mu.Unlock()

	// Tái sử dụng abortWithEvent trong quá trình tắt máy (đang chạy→tạm dừng + setAborting + Abort + sự kiện) và thủ công
	// Tạm dừng trình tự tương tự mà không sao chép lại; chỉ đặt dấu khi dừng (nhàn rỗi/tạm dừng) và tiếp tục chạy sau khi lập kế hoạch.
	if running {
		h.abortWithEvent("Bước vào giai đoạn đồng sáng tạo, sáng tạo đã bị đình chỉ", "info")
	} else {
		h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Bước vào giai đoạn đồng sáng tạo", Level: "info"})
	}
	return true
}

// ResumeFromCoCreate Giai đoạn kết thúc của quá trình đồng sáng tạo: Đưa hướng tiếp theo của đầu ra đồng sáng tạo dưới dạng can thiệp và tiếp tục quá trình tạo.
// Tái sử dụng đường dẫn tắt máy của Tiếp tục sau khi xóa dấu sử dụng (tuân theo các ràng buộc trước về ngân sách).
// Lưu ý: Trả sớm và đánh dấu không rõ khi trống bản nháp là cố ý (đồng sáng tạo vẫn chưa kết thúc); Phía TUI bảo vệ canStart()
// Tiêu chí "không trống" tương tự được sử dụng ở đây để đảm bảo rằng đường dẫn không thể truy cập được và quá trình tạo mã sẽ không bị rò rỉ do điều này.
func (h *Host) ResumeFromCoCreate(draft string) error {
	draft = strings.TrimSpace(draft)
	if draft == "" {
		return fmt.Errorf("draft is required")
	}
	h.mu.Lock()
	if !h.cocreating {
		h.mu.Unlock()
		return fmt.Errorf("not in co-create")
	}
	h.cocreating = false
	h.mu.Unlock()

	// Việc hủy bỏ PauseForCoCreate không đồng bộ: đợi lần chạy cũ hội tụ trước khi tiếp tục và tiếp tục sau khi tạm dừng thủ công.
	// Các điều kiện "tắt thực sự" nhất quán để tránh đưa lệnh tiếp tục vào quá trình chạy cũ đang thoát. Đồng sáng tạo trạng thái không hoạt động (chưa
	// Hủy bỏ), điều phối viên đã rảnh và WaitForIdle sẽ quay lại ngay lập tức.
	h.coordinator.WaitForIdle()

	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Quá trình đồng sáng tạo theo giai đoạn đã hoàn tất, hướng tiếp theo đã được đưa vào và quá trình tạo đã được tiếp tục", Level: "info"})
	return h.Continue(stagePlanPrefix + draft)
}

// CancelCoCreate Bỏ đồng tạo giai đoạn: xóa dấu nghề nghiệp và duy trì trạng thái tạm dừng (người dùng có thể tiếp tục trong hộp nhập liệu hoặc khởi động lại Resume).
func (h *Host) CancelCoCreate() {
	h.mu.Lock()
	if !h.cocreating {
		h.mu.Unlock()
		return
	}
	h.cocreating = false
	h.mu.Unlock()
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Giai đoạn đồng sáng tạo đã thoát và quá trình tạo vẫn bị tạm dừng (có thể tiếp tục trong hộp nhập liệu)", Level: "info"})
}

// ── Công cụ ──

func (h *Host) refreshWriterRestore() {
	if h.writerRestore != nil {
		h.writerRestore.Refresh(h.store)
	}
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// ImportFrom bắt đầu nhập ngược một tiểu thuyết bên ngoài: phân đoạn → nền tảng đảo ngược → phân tích vị trí từng chương.
// Loại trừ lẫn nhau với Điều phối viên; người gọi có thể Resume() ngay sau khi quá trình nhập hoàn tất.
// Kênh sự kiện được trả về sẽ bị đóng bởi imp.Run và người gọi có trách nhiệm sử dụng nó (loại bỏ nó nếu nó đầy để tránh chặn coroutine phân tích).
func (h *Host) ImportFrom(ctx context.Context, opts imp.Options) (<-chan imp.Event, error) {
	if err := h.guardExclusive("nhập khẩu"); err != nil {
		return nil, err
	}

	rulesOpts := rules.DefaultOptions(h.bundle.RulesFS)
	deps := imp.Deps{
		Store:      h.store,
		CommitTool: tools.NewCommitChapterTool(h.store).WithRules(rulesOpts),
		LLM:        h.models.ForRole("architect"),
		Prompts: imp.Prompts{
			Foundation: h.bundle.Prompts.ImportFoundation,
			Analyzer:   h.bundle.Prompts.ImportAnalyzer,
		},
	}
	return imp.Run(ctx, deps, opts)
}

// Mô phỏng đọc thư mục mô phỏng và tạo hoặc cập nhật dần dần chân dung mô phỏng.
func (h *Host) Simulate(ctx context.Context) (<-chan sim.Event, error) {
	if err := h.guardExclusive("Tạo chân dung mô phỏng"); err != nil {
		return nil, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working dir: %w", err)
	}
	deps := sim.Deps{
		Store: h.store,
		LLM:   h.models.ForRole("architect"),
		Prompts: sim.Prompts{
			Source: h.bundle.Prompts.SimulationSource,
			Merge:  h.bundle.Prompts.SimulationMerge,
		},
	}
	return sim.Run(ctx, deps, sim.Options{SourceDir: filepath.Join(wd, "simulate")})
}

// ImportSimulationProfile nhập chân dung mô phỏng được tạo trước đó.
func (h *Host) ImportSimulationProfile(ctx context.Context, path string) (<-chan sim.Event, error) {
	if err := h.guardExclusive("Nhập ảnh chân dung mô phỏng"); err != nil {
		return nil, err
	}
	return sim.RunImport(ctx, h.store, path)
}

// GuardExclusive kiểm tra khả năng chiếm giữ độc quyền: từ chối các mục nhập sẽ ghi đè trạng thái trong khi điều phối viên đang chạy hoặc trong cửa sổ đồng tạo giai đoạn
// (nhập/mô phỏng). Bù đắp cho khoảng cách đồng thời của việc chỉ kiểm tra ==chạy trong khi bị tạm dừng.
func (h *Host) guardExclusive(action string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.lifecycle == lifecycleRunning:
		return fmt.Errorf("Điều phối viên đang chạy, vui lòng tạm dừng trước rồi đến %s", action)
	case h.cocreating:
		return fmt.Errorf("Quá trình đồng sáng tạo theo giai đoạn đang được tiến hành. Vui lòng kết thúc đồng sáng tạo trước %s", action)
	}
	return nil
}

// Xuất xuất các chương đã hoàn thành sang các tệp bên ngoài (hiện chỉ hỗ trợ TXT).
//
// Khác với ImportFrom: xuất là thao tác chỉ đọc (không có Tiến trình/Điểm kiểm tra nào được di chuyển),
// Vì vậy, **Điều phối viên không bắt buộc phải nhàn rỗi** - "thành phẩm hiện tại" có thể được xuất bất cứ lúc nào trong quá trình viết.
// Chỉ đọc Progress.CompletedChapters + chương cuối cùng + dàn ý + ảnh chụp nhanh nhất quán của tiền đề.
func (h *Host) Export(ctx context.Context, opts exp.Options) (*exp.Result, error) {
	return exp.Run(ctx, exp.Deps{Store: h.store}, opts)
}
