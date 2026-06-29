package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const configDirName = ".ainovel"

// DefaultConfigPath trả về đường dẫn tệp cấu hình chung ~/.ainovel/config.json.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDirName, "config.json")
}

// DefaultConfigDir trả về đường dẫn thư mục ~/.ainovel; nếu không thể lấy được thư mục chính, một chuỗi trống sẽ được trả về.
// Chỉ được sử dụng để đọc/ghi các tệp không bắt buộc phải tồn tại (chẳng hạn như bộ đệm mô hình), thư mục sẽ không được tạo tự động.
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDirName)
}

// configDir trả về đường dẫn thư mục ~/.ainovel, đường dẫn này sẽ được tạo nếu nó không tồn tại.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, configDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return dir, nil
}

// projectConfigPath trả về đường dẫn tương đối của tệp cấu hình cấp dự án ./.ainovel/config.json.
// Dotdir cấp dự án phản chiếu toàn cầu ~/.ainovel/, sử dụng lại cùng configDirName; liên quan đến độ phân giải cwd.
func projectConfigPath() string {
	return filepath.Join(configDirName, "config.json")
}

// LoadConfig tải và hợp nhất các cấu hình theo mức độ ưu tiên:
//  1. ~/.ainovel/config.json (toàn cầu)
//  2. ./.ainovel/config.json (phạm vi cấp dự án)
//  3. Đường dẫn được chỉ định bởi flagPath (mức độ ưu tiên cao nhất)
func LoadConfig(flagPath string) (Config, error) {
	var cfg Config

	// 1. Cấu hình toàn cầu. Đây là cơ sở có mức độ ưu tiên thấp nhất, các tệp xấu bị hạ cấp thành cảnh báo thay vì bị chặn - có thể bị chặn theo cấp độ dự án
	//    / --config ghi đè; lỗi cứng sẽ khóa người dùng với "bad toàn cầu + hợp lệ --config",
	//    Vi phạm ngữ nghĩa "Tôi đã chỉ định rõ ràng điều này" của --config.
	if p := DefaultConfigPath(); p != "" {
		global, found, err := loadOptionalJSON(p)
		switch {
		case err != nil:
			slog.Warn("Phân tích cú pháp cấu hình chung không thành công và bị bỏ qua (có thể bị ghi đè bởi cấp dự án/--config)", "module", "config", "path", p, "err", err)
		case found:
			cfg = global
		}
	}

	// 2. Phạm vi cấp dự án. File bad bị lỗi lớn: các cấu hình mà người dùng chủ động cho vào thư mục hiện tại, âm thầm nuốt chửng sẽ gây ra
	//    Không có cách nào để khắc phục sự cố "cấu hình không có hiệu lực" (vấn đề #37).
	project, found, err := loadOptionalJSON(projectConfigPath())
	if err != nil {
		return cfg, fmt.Errorf("Cấu hình cấp dự án ..ainovel/config.json phân tích cú pháp không thành công (vui lòng kiểm tra cú pháp JSON): %w", err)
	}
	if found {
		cfg = mergeConfig(cfg, project)
	}

	// 3. Bảo hiểm cờ CLI
	if flagPath != "" {
		override, err := loadJSONFile(flagPath)
		if err != nil {
			return cfg, fmt.Errorf("load config %s: %w", flagPath, err)
		}
		cfg = mergeConfig(cfg, override)
	}

	return cfg, nil
}

// LoadOptionalJSON đọc tệp cấu hình tùy chọn:
//   - Tệp không tồn tại → (không, sai, không), giá trị mặc định/giá trị trên được xác định bởi người gọi
//   - File đã tồn tại nhưng phân tích không thành công → trả về lỗi (không còn âm thầm nuốt - nếu không cấu hình của người dùng "sẽ không có hiệu lực"
//     Nhưng không có cách nào khắc phục được, đây là nguyên nhân cốt lõi của vấn đề #37)
func loadOptionalJSON(path string) (Config, bool, error) {
	cfg, err := loadJSONFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	return cfg, true, nil
}

// LoadConfigFile đọc một tệp cấu hình JSON duy nhất và hỗ trợ nhận xét dòng //.
// Không có việc hợp nhất nào được thực hiện, chỉ có cấu hình của chính tệp đó được trả về. Trả về lỗi nếu tập tin không tồn tại.
func LoadConfigFile(path string) (Config, error) {
	return loadJSONFile(path)
}

// LoadJSONFile đọc các tệp cấu hình JSON và hỗ trợ nhận xét dòng //.
// Trả về lỗi nếu tệp không tồn tại (bỏ qua theo quyết định của người gọi).
func loadJSONFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cleaned := stripJSONComments(data)
	var cfg Config
	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// mergeConfig hợp nhất lớp phủ vào cơ sở. Các trường giá trị khác 0 được bao phủ và bản đồ được hợp nhất theo khóa.
func mergeConfig(base, overlay Config) Config {
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	if overlay.ModelName != "" {
		base.ModelName = overlay.ModelName
	}
	if overlay.Thinking != "" {
		base.Thinking = overlay.Thinking
	}
	if overlay.Style != "" {
		base.Style = overlay.Style
	}
	if overlay.ContextWindow > 0 {
		base.ContextWindow = overlay.ContextWindow
	}

	// Nhà cung cấp: Khóa lớp phủ bao gồm khóa cơ sở có cùng tên.
	if len(overlay.Providers) > 0 {
		if base.Providers == nil {
			base.Providers = make(map[string]ProviderConfig)
		}
		for k, v := range overlay.Providers {
			existing := base.Providers[k]
			if v.Type != "" {
				existing.Type = v.Type
			}
			if v.APIKey != "" {
				existing.APIKey = v.APIKey
			}
			if v.BaseURL != "" {
				existing.BaseURL = v.BaseURL
			}
			if len(v.Models) > 0 {
				existing.Models = append([]string(nil), v.Models...)
			}
			if len(v.ExtraBody) > 0 {
				existing.ExtraBody = cloneMap(v.ExtraBody)
			}
			if len(v.Extra) > 0 {
				existing.Extra = cloneMap(v.Extra)
			}
			base.Providers[k] = existing
		}
	}

	// Vai trò: Khóa lớp phủ bao gồm khóa cơ sở có cùng tên.
	if len(overlay.Roles) > 0 {
		if base.Roles == nil {
			base.Roles = make(map[string]RoleConfig)
		}
		for k, v := range overlay.Roles {
			existing := base.Roles[k]
			if v.Provider != "" {
				existing.Provider = v.Provider
			}
			if v.Model != "" {
				existing.Model = v.Model
			}
			if len(v.Fallbacks) > 0 {
				existing.Fallbacks = append([]ModelRef(nil), v.Fallbacks...)
			}
			if v.Thinking != "" {
				existing.Thinking = v.Thinking
			}
			base.Roles[k] = existing
		}
	}

	// Ngân sách/Thông báo: phạm vi bao phủ đầy đủ (ngân sách/báo động cấp dự án là một tuyên bố chính sách độc lập và không được ghép nối với từng lĩnh vực toàn cầu)
	if overlay.Budget != (BudgetConfig{}) {
		base.Budget = overlay.Budget
	}
	if overlay.Notify.Enabled != nil || overlay.Notify.Command != "" || len(overlay.Notify.Events) > 0 {
		base.Notify = overlay.Notify
	}

	return base
}

func cloneMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// StripJSONComments xóa // nhận xét dòng trong JSON và theo dõi trạng thái dấu ngoặc kép để tránh vô tình xóa nội dung chuỗi.
func stripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		b := data[i]

		if escaped {
			out = append(out, b)
			escaped = false
			continue
		}

		if inString {
			out = append(out, b)
			if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}

		// không nằm trong chuỗi
		if b == '"' {
			inString = true
			out = append(out, b)
			continue
		}

		// Phát hiện // chú thích
		if b == '/' && i+1 < len(data) && data[i+1] == '/' {
			// nhảy đến cuối dòng
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				out = append(out, '\n')
			}
			continue
		}

		out = append(out, b)
	}

	return out
}

// WriteStartupError Thêm các lỗi nghiêm trọng trong quá trình khởi động vào ~/.ainovel/last-error.log và trả về
// Đường dẫn tệp (nỗ lực tốt nhất, trả về chuỗi trống khi thất bại). Khi nhấn đúp để bắt đầu, cửa sổ console sẽ thực hiện theo quy trình
// Lối ra ngay lập tức bị đóng lại, lỗi biến mất trong nháy mắt và việc đặt hàng là cách duy nhất để những người dùng đó truy tìm lại sau này.
func WriteStartupError(msg string) string {
	dir := DefaultConfigDir()
	if dir == "" {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(dir, "last-error.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), msg); err != nil {
		return ""
	}
	return path
}

// SaveConfig ghi cấu hình vào đường dẫn đã chỉ định (định dạng JSON, thụt lề và làm đẹp).
func SaveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
