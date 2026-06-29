package bootstrap

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/voocel/agentcore/llm"
	"github.com/nguyenducvuongg/ainovelViet/internal/errs"
	"github.com/nguyenducvuongg/ainovelViet/internal/models"
	"github.com/nguyenducvuongg/ainovelViet/internal/utils"
)

// DefaultContextWindow Kích thước cửa sổ mặc định khi mô hình chưa được đăng ký trong sổ đăng ký.
const DefaultContextWindow = 200000

// CompactRatio Ngưỡng tương đối kích hoạt nén ngữ cảnh: nén khi mã thông báo >= window * CompactRatio.
// 0,85 là giá trị kinh nghiệm, để lại 15% khoảng trống trong đầu cho "vòng nhắc nhở tiếp theo + kết quả công cụ lớn", đồng thời cho phép một cửa sổ lớn
// Mô hình cũng có thể được nén chủ động ở mức 85% để tránh bị choáng ngợp (vùng mờ dần chú ý) trong cửa sổ danh nghĩa 1M.
//
// Không lộ cấu hình người dùng: có cùng nguồn gốc với context_window đã xóa - cho phép người dùng điều chỉnh
// Nếu núm kỹ thuật số nhảy ngang liên tục, tốt hơn hết bạn nên cố định giá trị hợp lý trong mã.
const CompactRatio = 0.85

// MinCompactReserve là giới hạn dưới của ReserveTokens. Mô hình cửa sổ nhỏ (chẳng hạn như 32k local qwen3:8b)
// Theo tỷ lệ 0,15, mức dự trữ chỉ là 4800 và một phản hồi của công cụ commit_chapter có thể lưu trữ 5-8k.
// Văn bản chương 8-15k——"Sau khi nhấn sẽ bị vượt ngay" sẽ xuất hiện. 8000 đảm bảo bộ đệm nửa bánh trong trường hợp xấu nhất.
const MinCompactReserve = 8000

// CompactReserveTokens Tính toán lại ReserveTokens theo CompactRatio và áp dụng sàn MinCompactReserve:
//
//	threshold = window - reserve = window * CompactRatio
//	reserve   = max(MinCompactReserve, window * (1 - CompactRatio))
//
// Để sử dụng với EngineConfig.ReserveTokens của Agentcore.context.Engine.
func CompactReserveTokens(window int) int {
	if window <= 0 {
		return 0
	}
	reserve := window - int(float64(window)*CompactRatio)
	if reserve < MinCompactReserve {
		return MinCompactReserve
	}
	return reserve
}

// ProviderConfig xác định thông tin xác thực của một nhà cung cấp LLM duy nhất.
type ProviderConfig struct {
	Type    string   `json:"type,omitempty"`     // Loại giao thức API (openai/anthropic/gemini), được chỉ định khi tùy chỉnh proxy
	APIKey  string   `json:"api_key,omitempty"`  // API Key
	BaseURL string   `json:"base_url,omitempty"` // API Base URL
	Models  []string `json:"models,omitempty"`   // Danh sách model tùy chọn để hiển thị khi TUI chuyển đổi
	// ExtraBody chuyển các tham số bổ sung cho nhà cung cấp một cách minh bạch cho từng yêu cầu (chẳng hạn như nhiệt độ/top_p/min_p/
	// Hiện diện_penalty hoặc các khóa dành riêng cho nhà cung cấp, chẳng hạn như chat_template_kwargs của nvidia trên think).
	// Các phần cuối của khả năng tương thích OpenAI được tích hợp nguyên văn vào phần nội dung yêu cầu (tức là quy ước extra_body); các giá trị là trách nhiệm của người dùng.
	ExtraBody map[string]any `json:"extra_body,omitempty"`
	// Extra được chuyển một cách minh bạch tới cấu hình cấp nhà cung cấp (litellm.ProviderConfig.Extra) cho HTTP
	// Các tùy chọn lớp máy khách/vận chuyển như tiêu đề, user_agent, humanpic_beta, v.v.
	Extra map[string]any `json:"extra,omitempty"`
}

// RequiresAPIKey Trả về liệu nhà cung cấp này có phải định cấu hình api_key một cách rõ ràng hay không.
// Hiệp định:
// 1. ollama / nền tảng không cho phép chìa khóa;
// 2. Các cấu hình chỉ định rõ ràng Loại được coi là tác nhân tùy chỉnh và không được phép sử dụng khóa;
// 3. Các nhà cung cấp khác yêu cầu khóa theo mặc định và duy trì xác minh thận trọng giao diện lưu trữ chính thức.
func (pc ProviderConfig) RequiresAPIKey(name string) bool {
	switch name {
	case "ollama", "bedrock":
		return false
	}
	return pc.Type == ""
}

// ProviderType Trả về loại giao thức API hợp lệ.
// Loại rõ ràng được ưu tiên; nếu không thì tên nhà cung cấp bắt buộc phải có trong sổ đăng ký Litellm.
func (pc ProviderConfig) ProviderType(name string) (string, error) {
	if pc.Type != "" {
		return pc.Type, nil
	}
	if llm.IsProviderRegistered(name) {
		return name, nil
	}
	return "", fmt.Errorf("nhà cung cấp %q bị thiếu loại và không có trong danh sách các nhà cung cấp đã biết của Litellm: %w", name, errs.ErrConfig)
}

// ModelRef đại diện cho sự kết hợp nhà cung cấp/mô hình.
type ModelRef struct {
	Provider string `json:"provider"` // tên nhà cung cấp (nhập vào bản đồ Nhà cung cấp)
	Model    string `json:"model"`    // Tên model (được truyền đi một cách minh bạch, không có bất kỳ phân tích nào)
}

// RoleConfig xác định ghi đè mô hình cho một vai trò.
type RoleConfig struct {
	Provider  string     `json:"provider"`            // Tên nhà cung cấp chính (nhập trong bản đồ Nhà cung cấp)
	Model     string     `json:"model"`               // Tên model chính (được truyền đi một cách rõ ràng, không có bất kỳ phân tích nào)
	Fallbacks []ModelRef `json:"fallbacks,omitempty"` // Danh sách mô hình/nhà cung cấp dự phòng rõ ràng
	// Suy nghĩ Cường độ suy nghĩ của nhân vật (tắt/tối thiểu/thấp/trung bình/cao/xhigh/max), trống = kế thừa mặc định cấp cao nhất.
	// Nó được áp dụng sau khi được tác nhân xác minh.ParseThinkingLevel và giá trị bị bỏ qua được coi là trống.
	Thinking string `json:"thinking,omitempty"`
}

// tên vai trò được biết đến được hỗ trợ.
var knownRoles = map[string]bool{
	"coordinator": true,
	"architect":   true,
	"writer":      true,
	"editor":      true,
}

// Cấu hình cấu hình ứng dụng mới.
type Config struct {
	// Các trường thời gian chạy (không được tuần tự hóa thành JSON)
	OutputDir string `json:"-"` // thư mục gốc đầu ra

	// Cấu hình LLM mặc định
	Provider  string `json:"provider"` // Nhà cung cấp mặc định (nhập vào bản đồ Nhà cung cấp)
	ModelName string `json:"model"`    // Tên mẫu mặc định
	// Cường độ suy nghĩ mặc định ở mức cao nhất (tắt/tối thiểu/thấp/trung bình/cao/xcao/tối đa), trống = không có phạm vi bao phủ (kế thừa mặc định của mô hình/nhà cung cấp).
	// Dự phòng giá trị này khi tư duy không được cấu hình riêng cho vai trò.
	Thinking string `json:"thinking,omitempty"`

	// Cửa hàng thông tin xác thực của nhà cung cấp
	Providers map[string]ProviderConfig `json:"providers,omitempty"`

	// Phạm vi mô hình cấp độ nhân vật
	Roles map[string]RoleConfig `json:"roles,omitempty"`

	// Thông số tạo
	Style string `json:"style,omitempty"`

	// ContextWindow Kích thước cửa sổ được sử dụng để nén ngữ cảnh. Khi để trống (0), nó sẽ được tự động phân tích theo tên model:
	// Sổ đăng ký chạm vào cửa sổ thực của mô hình đã sử dụng nhưng bỏ lỡ DefaultContextWindow mặc định.
	// Cấu hình rõ ràng sẽ có hiệu lực trước tiên - được sử dụng để chỉ định cửa sổ thực cho các mô hình tùy chỉnh không thể tìm thấy trong sổ đăng ký.
	// Hoặc ghim mô hình cửa sổ lớn vào một giá trị nhỏ hơn để kích hoạt nén sớm (các cửa sổ danh nghĩa 1M thường có mức độ chú ý giảm ở mức 200k+).
	// Nó chỉ ảnh hưởng đến ngưỡng nén và không thay đổi độ dài yêu cầu thực tế của API LLM; người dùng chịu trách nhiệm định cấu hình giá trị.
	ContextWindow int `json:"context_window,omitempty"`

	// Ngân sách Chính sách ngân sách chi phí của một cuốn sách; chỉ được kích hoạt khi book_usd > 0.
	Budget BudgetConfig `json:"budget,omitzero"`

	// Thông báo cấu hình cảnh báo không giám sát; được bật theo mặc định (kênh hệ thống).
	Notify NotifyConfig `json:"notify,omitzero"`
}

// BudgetConfig là tuyên bố chính sách người dùng dành cho một ví sách. Tắt máy chéo dòng tương đương với người dùng tại thời điểm đó
// Hủy bỏ thủ công—Máy chủ chỉ thực thi thay mặt người dùng và không đánh giá hành vi của mô hình (Kiến trúc §10 Ranh giới hiến pháp).
type BudgetConfig struct {
	BookUSD   float64 `json:"book_usd,omitempty"`   // Bắt buộc phải kích hoạt; 0/mặc định = không giới hạn
	WarnRatio float64 `json:"warn_ratio,omitempty"` // Mực nước báo động, mặc định 0,8
	HardStop  bool    `json:"hard_stop,omitempty"`  // true=Dừng lại ngay sau khi vượt qua vạch; theo mặc định, hãy đợi cho đến khi tác vụ của đại lý phụ hiện tại kết thúc
}

// Đã bật Trả về liệu chính sách ngân sách có được bật hay không.
func (b BudgetConfig) Enabled() bool { return b.BookUSD > 0 }

// Cấu hình kênh cảnh báo không giám sát NotifyConfig.
type NotifyConfig struct {
	Enabled *bool    `json:"enabled,omitempty"` // Mặc định đúng (không có cấu hình kênh hệ thống)
	Command string   `json:"command,omitempty"` // Tùy chọn, thay thế kênh hệ thống sau khi cấu hình (vào đây để đẩy di động)
	Events  []string `json:"events,omitempty"`  // Tùy chọn, loại bộ lọc (run_end/repeat/ngân sách), được bật hoàn toàn theo mặc định
}

// IsEnabled trả về xem cảnh báo có được bật hay không (mặc định là true).
func (n NotifyConfig) IsEnabled() bool { return n.Enabled == nil || *n.Enabled }

// ValidateBase xác minh cấu hình cơ bản.
func (c *Config) ValidateBase() error {
	if err := validateConfigText("provider", c.Provider); err != nil {
		return err
	}
	if err := validateConfigText("model", c.ModelName); err != nil {
		return err
	}

	if c.Provider == "" {
		return fmt.Errorf("provider is required: %w", errs.ErrConfig)
	}
	if c.ModelName == "" {
		return fmt.Errorf("model is required: %w", errs.ErrConfig)
	}

	// Nhà cung cấp mặc định phải có thông tin xác thực
	pc, ok := c.Providers[c.Provider]
	if !ok {
		return fmt.Errorf("nhà cung cấp %q không định cấu hình thông tin xác thực trong nhà cung cấp; nếu nhà cung cấp bị ghi đè trong ..ainovel/config.json, thì nhà cung cấp.%s (bao gồm api_key/base_url) phải được khai báo cùng lúc. Bạn không thể chỉ thay đổi nhà cung cấp cấp cao nhất: %w", c.Provider, c.Provider, errs.ErrConfig)
	}
	if pc.RequiresAPIKey(c.Provider) && pc.APIKey == "" {
		return fmt.Errorf("provider %q has no api_key configured: %w", c.Provider, errs.ErrConfig)
	}
	if err := validateProviderConfigText(c.Provider, pc); err != nil {
		return err
	}
	for name, provider := range c.Providers {
		if err := validateConfigText("provider name", name); err != nil {
			return err
		}
		if err := validateProviderConfigText(name, provider); err != nil {
			return err
		}
	}

	// Xác minh mức độ phù hợp của vai trò
	for role, rc := range c.Roles {
		if err := validateConfigText("role name", role); err != nil {
			return err
		}
		if err := validateConfigText(fmt.Sprintf("role %q provider", role), rc.Provider); err != nil {
			return err
		}
		if err := validateConfigText(fmt.Sprintf("role %q model", role), rc.Model); err != nil {
			return err
		}
		if !knownRoles[role] {
			return fmt.Errorf("unknown role %q in roles config (valid: coordinator/architect/writer/editor): %w", role, errs.ErrConfig)
		}
		if rc.Provider == "" || rc.Model == "" {
			return fmt.Errorf("role %q must have both provider and model: %w", role, errs.ErrConfig)
		}
		if err := c.validateModelRef(
			fmt.Sprintf("role %q", role),
			ModelRef{Provider: rc.Provider, Model: rc.Model},
		); err != nil {
			return err
		}
		for i, fallback := range rc.Fallbacks {
			if err := validateConfigText(fmt.Sprintf("role %q fallback[%d] provider", role, i), fallback.Provider); err != nil {
				return err
			}
			if err := validateConfigText(fmt.Sprintf("role %q fallback[%d] model", role, i), fallback.Model); err != nil {
				return err
			}
			if err := c.validateModelRef(
				fmt.Sprintf("role %q fallback[%d]", role, i),
				fallback,
			); err != nil {
				return err
			}
		}
	}

	// Xác minh chính sách ngân sách
	if c.Budget.BookUSD < 0 {
		return fmt.Errorf("budget.book_usd must be >= 0: %w", errs.ErrConfig)
	}
	if c.Budget.Enabled() && (c.Budget.WarnRatio <= 0 || c.Budget.WarnRatio >= 1) {
		return fmt.Errorf("budget.warn_ratio must be in (0, 1): %w", errs.ErrConfig)
	}

	// Xác minh cấu hình cảnh báo
	if err := validateConfigText("notify.command", c.Notify.Command); err != nil {
		return err
	}
	for _, ev := range c.Notify.Events {
		if !knownNotifyEvents[ev] {
			return fmt.Errorf("unknown notify event %q (valid: run_end/repeat/budget): %w", ev, errs.ErrConfig)
		}
	}

	return nil
}

var knownNotifyEvents = map[string]bool{"run_end": true, "repeat": true, "budget": true}

func validateProviderConfigText(name string, pc ProviderConfig) error {
	fields := []struct {
		label string
		value string
	}{
		{label: fmt.Sprintf("provider %q type", name), value: pc.Type},
		{label: fmt.Sprintf("provider %q api_key", name), value: pc.APIKey},
		{label: fmt.Sprintf("provider %q base_url", name), value: pc.BaseURL},
	}
	for _, field := range fields {
		if err := validateConfigText(field.label, field.value); err != nil {
			return err
		}
	}
	for i, model := range pc.Models {
		if err := validateConfigText(fmt.Sprintf("provider %q models[%d]", name, i), model); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigText(name, value string) error {
	if utils.ContainsControl(value) {
		return fmt.Errorf("%s contains control character: %w", name, errs.ErrConfig)
	}
	return nil
}

// DefaultProviderConfig Trả về cấu hình thông tin xác thực của nhà cung cấp mặc định.
func (c *Config) DefaultProviderConfig() ProviderConfig {
	if c.Providers == nil {
		return ProviderConfig{}
	}
	return c.Providers[c.Provider]
}

// FillDefaults Điền các giá trị mặc định.
func (c *Config) FillDefaults() {
	if c.OutputDir == "" {
		c.OutputDir = filepath.Join("output", "novel")
	}
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}
	if c.Roles == nil {
		c.Roles = make(map[string]RoleConfig)
	}
	if c.Style == "" {
		c.Style = "default"
	}
	if c.Budget.Enabled() && c.Budget.WarnRatio == 0 {
		c.Budget.WarnRatio = 0.8
	}
}

// ContextWindowSource đánh dấu nguồn của giá trị cửa sổ để sử dụng ghi nhật ký/chẩn đoán.
type ContextWindowSource string

const (
	CtxWindowConfig   ContextWindowSource = "config"   // Tệp cấu hình context_window được chỉ định rõ ràng
	CtxWindowRegistry ContextWindowSource = "registry" // Lượt truy cập cơ bản OpenRouter
	CtxWindowDefault  ContextWindowSource = "default"  // Giữ bí mật (đại lý tùy chỉnh/mô hình không xác định)
)

// ResolveContextWindow Giải quyết cửa sổ hợp lệ được sử dụng bằng cách nén ngữ cảnh, theo thứ tự ưu tiên:
//  1. Tệp cấu hình ContextWindow > 0 → Sử dụng trực tiếp (mức ưu tiên cao nhất, có thể vượt quá cửa sổ thực của mô hình)
//  2. models.DefaultRegistry Truy vấn theo tên model (đường cơ sở OpenRouter + làm mới 24h)
//  3. Đi tới cuối DefaultContextWindow (proxy tùy chỉnh/mô hình không xác định)
//
// Lưu ý: Giá trị trả về chỉ được sử dụng để tính toán ngưỡng nén và sẽ không làm giảm độ dài yêu cầu thực tế mà API LLM có thể gửi.
func (c Config) ResolveContextWindow(modelName string) (int, ContextWindowSource) {
	if c.ContextWindow > 0 {
		return c.ContextWindow, CtxWindowConfig
	}
	if rw := models.DefaultRegistry().ResolveContextWindow(modelName); rw > 0 {
		return rw, CtxWindowRegistry
	}
	return DefaultContextWindow, CtxWindowDefault
}

// ResolveThinking trả về chuỗi ban đầu về cường độ tư duy hiệu quả của nhân vật (tắt/tối thiểu/thấp/trung bình/cao/xcao/tối đa hoặc trống).
// Ưu tiên: Cấp vai trò Vai trò[role]. Suy nghĩ → Suy nghĩ mặc định cấp cao nhất → "" (không ghi đè, sử dụng mặc định của mô hình/nhà cung cấp).
// Khi vai trò trống hoặc "mặc định", mặc định cấp cao nhất sẽ được lấy trực tiếp. Tính hợp pháp của giá trị được kiểm tra bởi các đại lý.ParseThinkingLevel.
func (c Config) ResolveThinking(role string) string {
	if role != "" && role != "default" {
		if rc, ok := c.Roles[role]; ok && rc.Thinking != "" {
			return rc.Thinking
		}
	}
	return c.Thinking
}

// LogContextWindowChoice In quyết định cửa sổ cho một vai trò. Lời nhắc cảnh báo được đưa ra khi source=default
// Mô hình này không có trong sổ đăng ký (cũng không được bao gồm trong OpenRouter) và việc nén ngữ cảnh tiếp theo sẽ dựa trên cửa sổ dưới cùng.
// Kích hoạt - Nếu cửa sổ thực tế của mô hình lớn hơn, nó có thể được chỉ định rõ ràng bằng context_window trong tệp cấu hình để tránh bị nén trước và mất lịch sử.
func LogContextWindowChoice(role, model string, window int, source ContextWindowSource) {
	attrs := []any{"module", "context", "role", role, "model", model, "window", window, "source", source}
	switch source {
	case CtxWindowDefault:
		slog.Warn("Các mô hình không được nhận dạng, sử dụng cửa sổ phụ trợ (không bao gồm tác nhân tùy chỉnh hoặc OpenRouter, có thể được chỉ định rõ ràng bằng context_window)", attrs...)
	case CtxWindowConfig:
		slog.Info("Cửa sổ ngữ cảnh (từ tệp cấu hình context_window)", attrs...)
	default:
		slog.Info("cửa sổ ngữ cảnh", attrs...)
	}
}

// CandidateModels trả về danh sách các mô hình có thể được chuyển đổi theo một nhà cung cấp nhất định.
// Ưu tiên sử dụng các model được nhà cung cấp khai báo rõ ràng; đồng thời bổ sung các mô hình nhà cung cấp đã xuất hiện trong cấu hình hiện tại.
func (c Config) CandidateModels(provider string) []string {
	if provider == "" {
		return nil
	}

	seen := make(map[string]bool)
	models := make([]string, 0, 4)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		models = append(models, model)
	}

	if pc, ok := c.Providers[provider]; ok {
		for _, model := range pc.Models {
			add(model)
		}
	}
	if c.Provider == provider {
		add(c.ModelName)
	}
	for _, rc := range c.Roles {
		if rc.Provider == provider {
			add(rc.Model)
		}
		for _, fallback := range rc.Fallbacks {
			if fallback.Provider == provider {
				add(fallback.Model)
			}
		}
	}
	return models
}

func (c Config) validateModelRef(owner string, ref ModelRef) error {
	if ref.Provider == "" || ref.Model == "" {
		return fmt.Errorf("%s must have both provider and model: %w", owner, errs.ErrConfig)
	}

	pc, ok := c.Providers[ref.Provider]
	if !ok {
		return fmt.Errorf("%s references provider %q which is not configured: %w", owner, ref.Provider, errs.ErrConfig)
	}
	if pc.RequiresAPIKey(ref.Provider) && pc.APIKey == "" {
		return fmt.Errorf("%s references provider %q which has no api_key: %w", owner, ref.Provider, errs.ErrConfig)
	}
	return nil
}
