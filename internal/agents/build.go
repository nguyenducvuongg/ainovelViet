package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/voocel/agentcore"
	corecontext "github.com/voocel/agentcore/context"
	"github.com/voocel/agentcore/llm"
	"github.com/voocel/agentcore/subagent"
	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/agents/ctxpack"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host/reminder"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/tools"
)

// logRulesLoaded in trạng thái tải quy tắc trong quá trình lắp ráp: thư mục quy tắc của cuốn sách này, nguồn thực sự đã đọc và giá trị hiệu quả kiểm tra số từ.
// Nếu tệp quy tắc được đặt sai đường dẫn, nó sẽ bị trình tải âm thầm bỏ qua và nguồn sẽ không được nhập vào LLM (chỉ hiển thị bảng /diag). Nếu tệp quy tắc được đặt sai đường dẫn, sẽ không có phản hồi.
// Trở ngại lớn nhất cho việc khắc phục sự cố của người dùng. Dòng nhật ký khởi động này hiển thị nhanh "đường dẫn sai/số từ không được đưa vào nội dung phía trước".
func logRulesLoaded(opts rules.LoadOptions) {
	b := rules.Merge(rules.Load(opts))
	words := "Chưa được đặt (không kiểm tra số từ)"
	if w := b.Structured.ChapterWords; w != nil {
		words = fmt.Sprintf("%d-%d", w.Min, w.Max)
	}
	slog.Info("Tải quy tắc",
		"Mục lục của các quy tắc của cuốn sách này", opts.ProjectRulesDir,
		"Đã tải nguồn", b.Sources,
		"Số từ của chương", words)
}

// AgentToRole bình thường hóa tên tác nhân phụ thành tên vai trò được ModelSet công nhận.
// Architect_short / Architect_long đều có chung cấu hình vai trò kiến ​​trúc sư.
// Đồng nghĩa với Host.agentRoleName. Bởi vì bản dựng và máy chủ không phụ thuộc vào nhau nên mỗi bản giữ một bản sao.
func agentToRole(name string) string {
	if strings.HasPrefix(name, "architect_") {
		return "architect"
	}
	return name
}

// subagentMaxRetries cung cấp giới hạn trên thử lại LLM thống nhất cho tất cả SubAgentConfig và CoĐiều phối viên.
// Chiến lược lùi: Chỉ mục 1s/2s/4s/8s/16s (tuân theo giới hạn trên maxDelay), ưu tiên dành cho máy chủ Retry-After.
// Sử dụng ToolsAreIdempotent=true để tạo luồng không hoạt động/503/jitter mạng ngắn hạn có thể thử lại
// Các lỗi có thể được thử lại gần đó ở lớp tác nhân phụ thay vì ném toàn bộ tác nhân phụ trở lại điều phối viên để phân phối lại.
// Quy tắc cốt lõi của dự án là đảm bảo rằng các công cụ viết sử dụng điểm kiểm tra + thông báo bình thường và việc thử lại là an toàn.
const subagentMaxRetries = 5

// UsageRecorder là lệnh gọi lại sử dụng tùy chọn của BuildCoorder; chữ ký nhất quán với OnMessage.
// Mỗi thông điệp tác nhân được gửi đi một lần và lớp Máy chủ chịu trách nhiệm tổng hợp. nil có nghĩa là không theo dõi.
type UsageRecorder func(agentName string, msg agentcore.AgentMessage)

// FlowBoundaryHook runs synchronously after a Coordinator tool that advances
// the durable story state succeeds. Host uses it to queue the next flow
// instruction before the Coordinator gets another LLM turn.
type FlowBoundaryHook func(toolName string)

// ApplyThinking áp dụng cường độ tư duy của một nhân vật cụ thể cho tác nhân trực tiếp (để điều chỉnh thời gian chạy/mô hình).
// điều phối viên → Agent.SetThinkingLevel; kiến trúc sư → hai đại lý phụ Architect_*;
// người viết/người biên tập → tương ứng với tác nhân phụ. Cấp độ trống = kế thừa mô hình/mặc định của nhà cung cấp. Các tên vai trò khác bị bỏ qua.
type ApplyThinking func(role string, level agentcore.ThinkingLevel)

// ParseThinkingLevel chuyển đổi chuỗi cấu hình thành Agentcore.ThinkingLevel.
// "" là hợp pháp (= không ghi đè/kế thừa); phần còn lại phải ở mức tắt/tối thiểu/thấp/trung bình/cao/xcao/tối đa,
// Nếu không, lỗi sẽ được trả về (trống và cảnh báo khi hạ cấp khi khởi động và lỗi sẽ được lặp lại cho người dùng khi chạy).
func ParseThinkingLevel(s string) (agentcore.ThinkingLevel, error) {
	lv := agentcore.NormalizeThinkingLevel(agentcore.ThinkingLevel(s))
	switch lv {
	case "", agentcore.ThinkingOff, agentcore.ThinkingMinimal, agentcore.ThinkingLow,
		agentcore.ThinkingMedium, agentcore.ThinkingHigh, agentcore.ThinkingXHigh,
		agentcore.ThinkingMax:
		return lv, nil
	default:
		return "", fmt.Errorf("Cường độ tư duy không hợp lệ %q (tùy chọn: tắt/tối thiểu/thấp/trung bình/cao/xcao/tối đa)", s)
	}
}

func ResolveThinkingForModel(model agentcore.ChatModel, level agentcore.ThinkingLevel) (agentcore.ThinkingLevel, bool) {
	return llm.ThinkingPolicyFor(model).Resolve(level)
}

func AvailableThinkingForModel(model agentcore.ChatModel) []agentcore.ThinkingLevel {
	return llm.ThinkingPolicyFor(model).Available
}

// roleThinking phân tích cường độ tư duy hiệu quả của một vai trò nhất định; các giá trị không hợp lệ sẽ bị hạ cấp xuống mức trống (không bị ghi đè) và bị cảnh báo.
func roleThinking(cfg bootstrap.Config, role string) agentcore.ThinkingLevel {
	lv, err := ParseThinkingLevel(cfg.ResolveThinking(role))
	if err != nil {
		slog.Warn("Bỏ qua cấu hình cường độ suy nghĩ không hợp lệ", "module", "agent", "role", role, "err", err)
		return ""
	}
	return lv
}

func resolvedRoleThinking(model agentcore.ChatModel, cfg bootstrap.Config, role string) agentcore.ThinkingLevel {
	resolved, _ := ResolveThinkingForModel(model, roleThinking(cfg, role))
	return resolved
}

// BuildCogorator tập hợp Tác nhân điều phối và Tác nhân phụ của nó.
// Trả về tham chiếu ContextEngine của Tác nhân, AskUserTool, WriterRestorePack, Điều phối viên,
// Và việc đóng ApplyThinking - bạn cần gọi trực tiếp SetContextWindow + khi chuyển đổi lớp/mô hình máy chủ
// SetReserveTokens liên kết cửa sổ của mô hình mới (nhà văn/kiến trúc sư/biên tập viên truy cập ContextManagerFactory
// Tự động xây dựng lại, không cần ref; chỉ điều phối viên thường trú mới cần nó) và liên kết các vai trò khác nhau thông qua ApplyThinking
// Hãy suy nghĩ cường độ. Lớp Máy chủ nhận được luồng sự kiện thông qua Agent.Subscribe và không còn yêu cầu gọi lại phát ra nữa.
func BuildCoordinator(
	cfg bootstrap.Config,
	store *store.Store,
	models *bootstrap.ModelSet,
	bundle assets.Bundle,
	recordUsage UsageRecorder,
	onFlowBoundary FlowBoundaryHook,
) (*agentcore.Agent, *tools.AskUserTool, *ctxpack.WriterRestorePack, *corecontext.ContextEngine, ApplyThinking) {
	// Công cụ chia sẻ
	rulesOpts := rules.DefaultOptions(bundle.RulesFS)
	logRulesLoaded(rulesOpts)
	contextTool := tools.NewContextTool(store, bundle.References, cfg.Style, rulesOpts)
	readChapter := tools.NewReadChapterTool(store)
	askUser := tools.NewAskUserTool()

	architectTools := []agentcore.Tool{
		contextTool,
		tools.NewSaveFoundationTool(store),
	}
	writerTools := []agentcore.Tool{
		contextTool,
		readChapter,
		tools.NewPlanChapterTool(store),
		tools.NewDraftChapterTool(store),
		tools.NewEditChapterTool(store),
		tools.NewCheckConsistencyTool(store),
		tools.NewCommitChapterTool(store).WithRules(rulesOpts),
	}
	editorTools := []agentcore.Tool{
		contextTool,
		readChapter,
		tools.NewSaveReviewTool(store),
		tools.NewSaveArcSummaryTool(store),
		tools.NewSaveVolumeSummaryTool(store),
	}

	// Chuyển đổi dự phòng của nhà cung cấp chỉ ghi nhật ký và không thông báo cho máy chủ
	reportFailover := func(ev bootstrap.FailoverEvent) {
		slog.Warn("chuyển đổi nhà cung cấp",
			"module", "agent",
			"role", ev.Role,
			"reason", ev.Reason,
			"from", fmt.Sprintf("%s/%s", ev.FromProvider, ev.FromModel),
			"to", fmt.Sprintf("%s/%s", ev.ToProvider, ev.ToModel),
			"err", ev.Err,
		)
	}

	architectModel := models.ForRoleWithFailover("architect", reportFailover)
	writerModel := models.ForRoleWithFailover("writer", reportFailover)
	editorModel := models.ForRoleWithFailover("editor", reportFailover)
	coordinatorModel := models.ForRoleWithFailover("coordinator", reportFailover)

	// Trình quản lý bối cảnh của Điều phối viên được tạo một lần khi Tác nhân được xây dựng và phân tích cú pháp theo mô hình khởi động.
	// Khi chạy /model để chuyển sang mô hình có cửa sổ nhỏ hơn, người dùng nên cấu hình rõ ràng context_window.
	_, coordinatorModelName, _ := models.CurrentSelection("coordinator")
	coordinatorContextWindow, coordinatorSource := cfg.ResolveContextWindow(coordinatorModelName)
	// Trình quản lý bối cảnh của Writer được xây dựng lại với mỗi lệnh gọi đến nhà máy và cửa sổ sẽ tự động tuân theo quá trình hoán đổi mô hình (xem nhà máy bên dưới).
	_, writerModelName, _ := models.CurrentSelection("writer")
	writerContextWindow, writerSource := cfg.ResolveContextWindow(writerModelName)
	bootstrap.LogContextWindowChoice("coordinator", coordinatorModelName, coordinatorContextWindow, coordinatorSource)
	bootstrap.LogContextWindowChoice("writer", writerModelName, writerContextWindow, writerSource)

	// Khi modelLookup ghi vào phiên, hãy thêm _meta:{provider,model} vào từng tin nhắn trợ lý.
	// Điều này cho phép phát lại không còn dựa vào "ModelSet hiện tại" để suy ra chi phí lịch sử và nó có thể tính toán chính xác chi phí khi chuyển đổi mô hình trong quá trình vận hành.
	modelLookup := func(agentName string) (string, string) {
		role := agentToRole(agentName)
		provider, name, _ := models.CurrentSelection(role)
		return provider, name
	}
	baseOnMsg := store.Sessions.SubAgentLogger(modelLookup)
	onMsg := func(agentName, task string, msg agentcore.AgentMessage) {
		baseOnMsg(agentName, task, msg)
		if recordUsage != nil {
			recordUsage(agentName, msg)
		}
	}
	baseCoordinatorLog := store.Sessions.CoordinatorLogger(modelLookup)
	coordinatorOnMessage := func(msg agentcore.AgentMessage) {
		baseCoordinatorLog(msg)
		if recordUsage != nil {
			recordUsage("coordinator", msg)
		}
	}

	architectStopGuardFactory := func(_, _ string) agentcore.StopGuard {
		return reminder.NewArchitectStopGuard(store)
	}
	architectThinking, _ := ResolveThinkingForModel(architectModel, roleThinking(cfg, "architect"))
	architectShort := subagent.Config{
		Name:               "architect_short",
		Description:        "Công cụ lập kế hoạch truyện ngắn: Tạo các cài đặt nhỏ gọn và dàn ý phẳng cho các câu chuyện một tập, xung đột đơn, mật độ cao",
		Model:              architectModel,
		SystemPrompt:       bundle.Prompts.ArchitectShort,
		Tools:              architectTools,
		MaxTurns:           15,
		MaxRetries:         subagentMaxRetries,
		ThinkingLevel:      architectThinking,
		ToolsAreIdempotent: true,
		OnMessage:          onMsg,
		StopAfterToolResult: func(toolName string, result json.RawMessage) bool {
			r := decodeSaveFoundationResult(toolName, result)
			return r.Type == "outline" && r.FoundationReady
		},
		StopGuardFactory: architectStopGuardFactory,
	}
	architectLong := subagent.Config{
		Name:                "architect_long",
		Description:         "Công cụ lập kế hoạch dạng dài: Tạo các cài đặt phân cấp và phác thảo vòng cung cho các câu chuyện nâng cấp bền vững, nối tiếp.",
		Model:               architectModel,
		SystemPrompt:        bundle.Prompts.ArchitectLong,
		Tools:               architectTools,
		MaxTurns:            20,
		MaxRetries:          subagentMaxRetries,
		ThinkingLevel:       architectThinking,
		ToolsAreIdempotent:  true,
		OnMessage:           onMsg,
		StopAfterToolResult: architectLongShouldStopAfterToolResult,
		StopGuardFactory:    architectStopGuardFactory,
	}

	writerPrompt := bundle.Prompts.Writer
	if style, ok := bundle.Styles[cfg.Style]; ok {
		writerPrompt += "\n\n" + style
	}

	restore := &ctxpack.WriterRestorePack{}
	restore.Refresh(store)

	writer := subagent.Config{
		Name:               "writer",
		Description:        "Tác giả: độc lập hoàn thiện việc lên ý tưởng, viết, tự nhận xét và nộp chương",
		Model:              writerModel,
		SystemPrompt:       writerPrompt,
		Tools:              writerTools,
		MaxTurns:           30,
		MaxRetries:         subagentMaxRetries,
		ThinkingLevel:      resolvedRoleThinking(writerModel, cfg, "writer"),
		ToolsAreIdempotent: true,
		StopAfterTools:     []string{"commit_chapter"},
		OnMessage:          onMsg,
		StopGuardFactory: func(_, _ string) agentcore.StopGuard {
			return reminder.NewWriterStopGuard(store)
		},
		ContextManagerFactory: func(model agentcore.ChatModel) agentcore.ContextManager {
			// Mỗi cuộc gọi tác nhân phụ (người viết) sẽ xây dựng lại, đọc tên mô hình mới nhất từ ​​runModel hiện tại.
			// /model tự động sử dụng cửa sổ mới cho chương tiếp theo sau khi chuyển đổi người viết.
			window, _ := cfg.ResolveContextWindow(bootstrap.ModelName(model))
			return newContextManager(contextManagerConfig{
				Model:            model,
				ContextWindow:    window,
				ReserveTokens:    bootstrap.CompactReserveTokens(window),
				KeepRecentTokens: 20000,
				Agent:            "writer",
				ToolMicrocompact: &corecontext.ToolResultMicrocompactConfig{
					IdleThreshold: 5 * time.Minute,
				},
				ExtraStrategies: []corecontext.Strategy{
					ctxpack.NewStoreSummaryCompact(ctxpack.StoreSummaryCompactConfig{
						Store:            store,
						KeepRecentTokens: 20000,
					}),
				},
				Summary: &corecontext.FullSummaryConfig{
					PostSummaryHooks:    []corecontext.PostSummaryHook{restore.Hook()},
					SystemPrompt:        ctxpack.WriterSummarySystemPrompt,
					SummaryPrompt:       ctxpack.WriterSummaryPrompt,
					UpdateSummaryPrompt: ctxpack.WriterUpdateSummaryPrompt,
					TurnPrefixPrompt:    ctxpack.WriterTurnPrefixPrompt,
				},
			})
		},
	}

	editor := subagent.Config{
		Name:               "editor",
		Description:        "Người đánh giá: Đọc văn bản gốc và tìm ra các vấn đề từ cả cấp độ cấu trúc và thẩm mỹ",
		Model:              editorModel,
		SystemPrompt:       bundle.Prompts.Editor,
		Tools:              editorTools,
		MaxTurns:           20,
		MaxRetries:         subagentMaxRetries,
		ThinkingLevel:      resolvedRoleThinking(editorModel, cfg, "editor"),
		ToolsAreIdempotent: true,
		OnMessage:          onMsg,
		// Chỉ những sản phẩm cuối cùng thuộc loại tóm tắt mới dừng khi nhấn; save_review sẽ không còn dừng cứng nữa - Lối thoát StopAfterTool sẽ bỏ qua
		// StopGuard (agentcore loop.go), nếu save_review bị dừng cứng, "tóm tắt cung được lấy nhưng được xem xét trước"
		// Trình chỉnh sửa sẽ bị cắt tại save_review và không thể truy cập save_arc_summary. đánh giá/bài tập tóm tắt
		// Việc đóng hiện được kiểm soát bởi NewEditorStopGuard nhận biết tác vụ.
		StopAfterToolResult: func(toolName string, _ json.RawMessage) bool {
			return toolName == "save_arc_summary" || toolName == "save_volume_summary"
		},
		StopGuardFactory: func(_, task string) agentcore.StopGuard {
			return reminder.NewEditorStopGuard(store, task)
		},
	}

	subagentTool := subagent.New(architectShort, architectLong, writer, editor)

	coordinatorEngine := newContextManager(contextManagerConfig{
		Model:            coordinatorModel,
		ContextWindow:    coordinatorContextWindow,
		ReserveTokens:    bootstrap.CompactReserveTokens(coordinatorContextWindow),
		KeepRecentTokens: 30000,
		Agent:            "coordinator",
		CommitOnProject:  true,
	})

	agent := agentcore.NewAgent(
		agentcore.WithModel(coordinatorModel),
		agentcore.WithSystemPrompt(bundle.Prompts.Coordinator),
		agentcore.WithTools(subagentTool, contextTool, tools.NewSaveDirectiveTool(store), tools.NewReopenBookTool(store)),
		agentcore.WithMaxTurns(100_000),
		agentcore.WithOnMessage(coordinatorOnMessage),
		agentcore.WithToolsAreIdempotent(true),
		// Tác nhân phụ là kênh chính của quy trình; các lỗi thực sự phải được trả lại rõ ràng cho Máy chủ thay vì vô hiệu hóa vĩnh viễn công cụ trong một lần chạy.
		agentcore.WithMaxToolErrors(0),
		agentcore.WithMaxRetries(subagentMaxRetries),
		agentcore.WithContextManager(coordinatorEngine),
		agentcore.WithStopGuard(reminder.NewStopGuard(store, nil)),
		agentcore.WithMiddlewares(flowBoundaryMiddleware(onFlowBoundary)),
		// Khi pha=hoàn thành, việc gửi tác nhân phụ sẽ bị chặn cứng để ngăn Trình ghi khỏi vòng lặp vô tận.
		agentcore.WithToolGate(combineToolGates(
			completePhaseGate(store),
			writerExpandedChapterGate(store),
		)),
	)
	// Cường độ tư duy của người điều phối: áp dụng kết quả phân tích một cách vô điều kiện. Nó trống khi không được cấu hình (không gửi suy nghĩ, sử dụng nhà cung cấp
	// Mặc định), nhất quán với từng tác nhân phụ (Config.ThinkingLevel trống theo mặc định) - tránh ghi đè mặc định lõi tác nhân
	// ThinkLow buộc tất cả các nhà cung cấp phải ở mức thấp (kể cả GLM/Ollama sẽ buộc phải suy nghĩ).
	coordinatorThinking, _ := ResolveThinkingForModel(models.ForRole("coordinator"), roleThinking(cfg, "coordinator"))
	agent.SetThinkingLevel(coordinatorThinking)

	// Cường độ tư duy của từng vai trò được liên kết trong thời gian chạy: điều phối viên đảm nhận vai trò Tác nhân và tác nhân phụ đảm nhận vai trò ghi đè subagentTool.
	applyThinking := func(role string, level agentcore.ThinkingLevel) {
		switch role {
		case "coordinator":
			level, _ = ResolveThinkingForModel(models.ForRole("coordinator"), level)
			agent.SetThinkingLevel(level)
		case "architect":
			level, _ = ResolveThinkingForModel(models.ForRole("architect"), level)
			subagentTool.SetThinkingLevel("architect_short", level)
			subagentTool.SetThinkingLevel("architect_long", level)
		case "writer", "editor":
			level, _ = ResolveThinkingForModel(models.ForRole(role), level)
			subagentTool.SetThinkingLevel(role, level)
		}
	}

	return agent, askUser, restore, coordinatorEngine, applyThinking
}

func flowBoundaryMiddleware(onBoundary FlowBoundaryHook) agentcore.ToolMiddleware {
	return func(ctx context.Context, call agentcore.ToolCall, next agentcore.ToolExecuteFunc) (json.RawMessage, error) {
		out, err := next(ctx, call.Args)
		if err == nil && onBoundary != nil && isFlowBoundaryTool(call.Name) {
			onBoundary(call.Name)
		}
		return out, err
	}
}

func isFlowBoundaryTool(name string) bool {
	return name == "subagent" || name == "reopen_book"
}

// CompletePhaseGate Trả về một ToolGate từ chối tất cả các tác nhân phụ được gửi đi khi giai đoạn = hoàn thành.
// Ngăn Điều phối viên LLM vẫn gọi Nhà văn/Kiến trúc sư sau khi cuốn sách được hoàn thành, gây ra vòng lặp vô hạn.
func completePhaseGate(st *store.Store) agentcore.ToolGate {
	return func(_ context.Context, req agentcore.GateRequest) (*agentcore.GateDecision, error) {
		if req.Call.Name != "subagent" {
			return nil, nil
		}
		// không mở được: Khi có lỗi Tải hoặc tiến trình trống, nó sẽ luôn được giải phóng và phân phối bình thường sẽ không bị chặn do lỗi đọc tức thời.
		// Cái giá duy nhất là sự bế tắc có thể tái diễn khi toàn bộ thời gian trùng với lỗi đọc (xác suất cực kỳ thấp và có thể chấp nhận được).
		progress, _ := st.Progress.Load()
		if progress != nil && progress.Phase == domain.PhaseComplete {
			return &agentcore.GateDecision{
				Allowed: false,
				Reason:  "Toàn bộ cuốn sách đã được hoàn thành (giai đoạn=hoàn thành) và không thể gửi trực tiếp các đại lý phụ. Nếu người dùng muốn làm lại các chương đã viết, trước tiên vui lòng gọi open_book(chapters=[...]) để mở lại sách và vào trạng thái làm lại (người viết sẽ tự động được cử đến để viết lại sau); nếu người dùng muốn thêm một ô mới, người dùng sẽ được thông báo rằng cần có một dự án mới.",
			}, nil
		}
		return nil, nil
	}
}

func combineToolGates(gates ...agentcore.ToolGate) agentcore.ToolGate {
	return func(ctx context.Context, req agentcore.GateRequest) (*agentcore.GateDecision, error) {
		for _, gate := range gates {
			if gate == nil {
				continue
			}
			decision, err := gate(ctx, req)
			if err != nil {
				return nil, err
			}
			if decision != nil && !decision.Allowed {
				return decision, nil
			}
		}
		return nil, nil
	}
}

func writerExpandedChapterGate(st *store.Store) agentcore.ToolGate {
	return func(_ context.Context, req agentcore.GateRequest) (*agentcore.GateDecision, error) {
		if req.Call.Name != "subagent" {
			return nil, nil
		}
		var args struct {
			Agent string `json:"agent"`
			Task  string `json:"task"`
		}
		if err := json.Unmarshal(req.Call.Args, &args); err != nil || args.Agent != "writer" {
			return nil, nil
		}
		chapter := chapterFromTask(args.Task)
		if chapter <= 0 {
			chapter = writerFallbackChapter(st)
		}
		if chapter <= 0 {
			return nil, nil
		}
		if err := tools.EnsureChapterExpanded(st, chapter); err != nil {
			return &agentcore.GateDecision{
				Allowed: false,
				Reason:  err.Error() + ". Thay vào đó, vui lòng gửi Architect_long, gọi save_foundation(type=expand_arc) để mở rộng phần tiếp theo hoặc type=append_volume để nối thêm và mở rộng tập tiếp theo trước khi cử người viết.",
			}, nil
		}
		return nil, nil
	}
}

func writerFallbackChapter(st *store.Store) int {
	if st == nil {
		return 0
	}
	progress, err := st.Progress.Load()
	if err != nil || progress == nil {
		return 0
	}
	if len(progress.PendingRewrites) > 0 {
		return progress.PendingRewrites[0]
	}
	return progress.NextChapter()
}

var chapterTaskRe = regexp.MustCompile(`Chương\s*(\d+)\s*`)

func chapterFromTask(task string) int {
	m := chapterTaskRe.FindStringSubmatch(task)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

type saveFoundationResult struct {
	Type            string `json:"type"`
	FoundationReady bool   `json:"foundation_ready"`
}

func decodeSaveFoundationResult(toolName string, result json.RawMessage) saveFoundationResult {
	if toolName != "save_foundation" {
		return saveFoundationResult{}
	}
	var r saveFoundationResult
	_ = json.Unmarshal(result, &r)
	return r
}

func architectLongShouldStopAfterToolResult(toolName string, result json.RawMessage) bool {
	r := decodeSaveFoundationResult(toolName, result)
	switch r.Type {
	case "expand_arc", "complete_book":
		return true
	default:
		return false
	}
}
