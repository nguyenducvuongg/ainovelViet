// Các mô hình gói cung cấp sổ đăng ký siêu dữ liệu mô hình LLM (cửa sổ ngữ cảnh, giới hạn đầu ra, giá),
// API OpenRouter nguồn dữ liệu, đường cơ sở thời gian biên dịch + làm mới thời gian chạy.
package models

//go:generate go run gen_models.go

import (
	"strings"
	"sync"
)

// ModelEntry mô tả một mô hình LLM đã biết.
type ModelEntry struct {
	Provider            string  `json:"provider"`               // Tên nhà cung cấp được chuẩn hóa OpenRouter (anthropic/openai/gemini/...)
	ID                  string  `json:"id"`                     // ID mẫu (không có tiền tố nhà cung cấp)
	Name                string  `json:"name"`                   // tên hiển thị
	ContextWindow       int     `json:"context_window"`         // cửa sổ nhập liệu
	MaxTokens           int     `json:"max_tokens"`             // Giới hạn trên đầu ra đơn
	InputCostPer1M      float64 `json:"input_cost_per_1m"`      // Nhập giá (USD/1 triệu token)
	OutputCostPer1M     float64 `json:"output_cost_per_1m"`     // giá đầu ra
	CacheReadCostPer1M  float64 `json:"cache_read_cost_per_1m"` // Giá đọc bộ nhớ đệm
	CacheWriteCostPer1M float64 `json:"cache_write_cost_per_1m"`
}

// ModelRegistry lưu các mô hình đã biết và hỗ trợ phân tích cú pháp mờ và hợp nhất thời gian chạy.
type ModelRegistry struct {
	mu     sync.RWMutex
	models []ModelEntry
}

// NewModelRegistry Trả về một sổ đăng ký có tải đường cơ sở tại thời điểm biên dịch.
func NewModelRegistry() *ModelRegistry {
	r := &ModelRegistry{}
	r.models = append(r.models, generatedModels...)
	return r
}

var (
	defaultRegistry     *ModelRegistry
	defaultRegistryOnce sync.Once
)

// DefaultRegistry trả về sổ đăng ký toàn cầu (tải chậm, an toàn theo luồng).
// Gọi StartPricingRefresh trong giai đoạn khởi động cho phép nền làm mới thông tin về giá/cửa sổ.
func DefaultRegistry() *ModelRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewModelRegistry()
	})
	return defaultRegistry
}

// Resolve tìm thấy các mục dựa trên mã định danh mô hình (có thể là "nhà cung cấp/kiểu máy", ID đầy đủ hoặc tên cục bộ).
//
// Thứ tự phù hợp:
//  1. Nếu nó chứa "/", hãy tìm kiếm chính xác theo "nhà cung cấp/model"
//  2. Kết hợp hậu tố chính xác/ngày
//  3. So khớp chuỗi con (ID hoặc Tên chứa mẫu)
//
// Khi có nhiều lượt truy cập, bí danh không có hậu tố ngày sẽ được trả về đầu tiên (ví dụ: claude-sonnet-4 được ưu tiên hơn claude-sonnet-4-20250514).
func (r *ModelRegistry) Resolve(pattern string) (*ModelEntry, bool) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if idx := strings.Index(pattern, "/"); idx > 0 {
		prov := pattern[:idx]
		modelID := pattern[idx+1:]
		if entry, ok := lookupModelEntry(r.models, prov, modelID); ok {
			return &entry, true
		}
		// Tiền tố nhà cung cấp của OpenRouter (google/, x-ai/) không nhất thiết phải bằng tên Nhà cung cấp địa phương.
		// Chỉ trả về truy vấn modelID để đảm bảo rằng "google/gemini-2.5-pro" có thể đạt được mục nhập gemini.
		if entry, ok := lookupModelEntry(r.models, "", modelID); ok {
			return &entry, true
		}
	}

	if entry, ok := lookupModelEntry(r.models, "", pattern); ok {
		return &entry, true
	}

	lower := strings.ToLower(pattern)
	normalized := normalizeModelLookupID(pattern)
	var candidates []int
	for i := range r.models {
		if strings.Contains(normalizeModelLookupID(r.models[i].ID), normalized) ||
			strings.Contains(strings.ToLower(r.models[i].ID), lower) ||
			strings.Contains(strings.ToLower(r.models[i].Name), lower) {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return nil, false
	}

	best := candidates[0]
	for _, i := range candidates[1:] {
		if !hasDatedSuffix(r.models[i].ID) && hasDatedSuffix(r.models[best].ID) {
			best = i
		}
	}
	entry := r.models[best]
	return &entry, true
}

// ResolveContextWindow Trả về cửa sổ ngữ cảnh cho một mô hình; trả về 0 khi bỏ lỡ.
func (r *ModelRegistry) ResolveContextWindow(pattern string) int {
	if e, ok := r.Resolve(pattern); ok {
		return e.ContextWindow
	}
	return 0
}

// Danh sách trả về tất cả các mô hình (bộ lọc tùy chọn, chuỗi trống có nghĩa là số đầy đủ).
func (r *ModelRegistry) List(filter string) []ModelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == "" {
		return append([]ModelEntry{}, r.models...)
	}
	lower := strings.ToLower(filter)
	normalized := normalizeModelLookupID(filter)
	var out []ModelEntry
	for _, m := range r.models {
		if strings.Contains(strings.ToLower(m.Provider), lower) ||
			strings.Contains(normalizeModelLookupID(m.ID), normalized) ||
			strings.Contains(strings.ToLower(m.ID), lower) ||
			strings.Contains(strings.ToLower(m.Name), lower) {
			out = append(out, m)
		}
	}
	return out
}

// MergeModels được hợp nhất không phân biệt chữ hoa chữ thường bởi nhà cung cấp+id.
// Giá/cửa sổ/MaxTokens/Tên khác 0 sẽ ghi đè các mục hiện có; các mục mới sẽ được thêm trực tiếp.
func (r *ModelRegistry) MergeModels(fetched []ModelEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := make(map[string]int, len(r.models))
	for i, m := range r.models {
		idx[strings.ToLower(m.Provider+"/"+m.ID)] = i
	}
	for _, f := range fetched {
		key := strings.ToLower(f.Provider + "/" + f.ID)
		if i, ok := idx[key]; ok {
			if f.InputCostPer1M > 0 || f.OutputCostPer1M > 0 {
				r.models[i].InputCostPer1M = f.InputCostPer1M
				r.models[i].OutputCostPer1M = f.OutputCostPer1M
				r.models[i].CacheReadCostPer1M = f.CacheReadCostPer1M
				r.models[i].CacheWriteCostPer1M = f.CacheWriteCostPer1M
			}
			if f.ContextWindow > 0 {
				r.models[i].ContextWindow = f.ContextWindow
			}
			if f.MaxTokens > 0 {
				r.models[i].MaxTokens = f.MaxTokens
			}
			if f.Name != "" {
				r.models[i].Name = f.Name
			}
		} else {
			r.models = append(r.models, f)
			idx[key] = len(r.models) - 1
		}
	}
}
