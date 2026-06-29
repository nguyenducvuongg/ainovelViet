package domain

import "time"

// UsageSchemaVersion là số phiên bản tương thích của meta/usage.json.
// Nếu ngữ nghĩa của trường AgentUsageTotals thay đổi trong tương lai, hãy tăng giá trị này; UsageStore.Load nên bỏ qua nó và kích hoạt việc xây dựng lại phát lại khi nhìn thấy các phiên bản khác nhau.
const UsageSchemaVersion = 2

// UsageState là một bản tóm tắt nhanh về việc sử dụng mã thông báo/chi phí tích lũy.
// Bộ nhớ được duy trì bởi UsageTracker và được chuyển đổi định kỳ thành meta/usage.json.
//
// Lưu ý: Các mẫu cửa sổ trượt ("tỷ lệ truy cập gần N") bên trong UsageTracker **không được tồn tại**——
// Nó chỉ phục vụ chẩn đoán ngắn hạn về giao diện người dùng và ngữ nghĩa có thể được khôi phục bằng cách khởi động lại quy trình và tích lũy nó trong một vài vòng từ lúc trống.
// MissingAssistantUsage vẫn tồn tại dai dẳng và sự tích lũy trong các lần khởi động lại có nhiều giá trị chẩn đoán hơn.
type UsageState struct {
	Schema       int                         `json:"schema"`
	UpdatedAt    time.Time                   `json:"updated_at"`
	Overall      AgentUsageTotals            `json:"overall"`
	PerAgent     map[string]AgentUsageTotals `json:"per_agent"`
	PerModel     map[string]AgentUsageTotals `json:"per_model,omitempty"`
	MissingUsage int                         `json:"missing_assistant_usage"`
}

// AgentUsageTotals là sự thể hiện liên tục về số lượng tích lũy của một vai trò (hoặc tổng thể).
type AgentUsageTotals struct {
	Input        int     `json:"input"`
	Output       int     `json:"output"`
	CacheRead    int     `json:"cache_read"`
	CacheWrite   int     `json:"cache_write"`
	Cost         float64 `json:"cost_usd"`
	Saved        float64 `json:"saved_usd"`
	CacheCapable bool    `json:"cache_capable"`
}
