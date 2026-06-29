package bootstrap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/voocel/ainovel-cli/internal/errs"
)

const validGlobal = `{
  "provider": "openrouter",
  "model": "google/gemini-2.5-flash",
  "providers": { "openrouter": { "api_key": "sk-test-123456" } }
}`

// writeGlobal ghi cấu hình chung dưới một HOME bị cô lập và trả về HOME đó.
func writeGlobal(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".ainovel")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o644); err != nil {
			t.Fatalf("write global: %v", err)
		}
	}
	return home
}

// writeProjectConfig ghi cấu hình cấp dự án trong ./.ainovel/ trong thư mục làm việc hiện tại.
// Bạn cần phải t.Chdir vào thư mục đích trước khi gọi.
func writeProjectConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(".ainovel", 0o755); err != nil {
		t.Fatalf("mkdir .ainovel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".ainovel", "config.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}
}

// Nguyên nhân cốt lõi 3: ./.ainovel/config.json cấp dự án tồn tại nhưng là JSON không hợp lệ. Một lỗi phải được báo cáo và không thể nuốt chửng và quay trở lại tình hình toàn cầu.
func TestLoadConfig_CorruptProjectFailsLoud(t *testing.T) {
	writeGlobal(t, validGlobal)
	proj := t.TempDir()
	t.Chdir(proj)
	// Ví dụ viết tay có thêm dấu phẩy ở cuối - dạng JSON xấu phổ biến nhất.
	writeProjectConfig(t, `{ "model": "x", }`)

	if _, err := LoadConfig(""); err == nil {
		t.Fatal("..ainovel/config.json không hợp lệ sẽ báo lỗi nhưng bị bỏ qua trong âm thầm")
	}
}

// Toàn cầu là cơ sở có mức ưu tiên thấp nhất: các tệp xấu không được chặn phần ghi đè --config có mức ưu tiên cao hơn (bảo vệ trả lại --
// Phiên bản trước bị lỗi to toàn cầu, khiến người dùng có "global + hợp lệ --config" bị chặn bởi các tệp không liên quan).
func TestLoadConfig_CorruptGlobalDoesNotBlockOverride(t *testing.T) {
	writeGlobal(t, `{ not json`)
	proj := t.TempDir()
	t.Chdir(proj)
	good := filepath.Join(proj, "good.json")
	if err := os.WriteFile(good, []byte(validGlobal), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}

	cfg, err := LoadConfig(good)
	if err != nil {
		t.Fatalf("Toàn cầu xấu không nên chặn --config hợp lệ, có: %v", err)
	}
	if cfg.Provider != "openrouter" {
		t.Errorf("Giá trị của --config nên được sử dụng, dẫn đến nhà cung cấp=%q", cfg.Provider)
	}
}

// Tình huống bình thường là tệp không tồn tại (di động/lần đầu tiên) và không thể báo cáo lỗi.
func TestLoadConfig_MissingFilesNoError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // ~/.ainovel/config.json không tồn tại
	t.Chdir(t.TempDir())   // Cũng không có ..ainovel/config.json

	if _, err := LoadConfig(""); err != nil {
		t.Fatalf("Thiếu file cấu hình không gây ra lỗi, ta nhận được: %v", err)
	}
}

// Đường dẫn thông thường: hợp nhất cấp độ dự án + toàn cầu có hiệu lực.
func TestLoadConfig_ValidMergeWorks(t *testing.T) {
	writeGlobal(t, validGlobal)
	proj := t.TempDir()
	t.Chdir(proj)
	writeProjectConfig(t, `{
  "model": "google/gemini-2.5-pro",
  "thinking": "high",
  "roles": {
    "writer": {
      "provider": "openrouter",
      "model": "google/gemini-2.5-flash",
      "thinking": "low"
    }
  }
}`)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Cấu hình hợp lệ sẽ không gây ra lỗi: %v", err)
	}
	if cfg.Provider != "openrouter" {
		t.Errorf("nhà cung cấp nên giữ lại openrouter giá trị toàn cầu, nhận %q", cfg.Provider)
	}
	if cfg.ModelName != "google/gemini-2.5-pro" {
		t.Errorf("mô hình nên được ghi đè ở cấp dự án, nhận %q", cfg.ModelName)
	}
	if cfg.Thinking != "high" {
		t.Errorf("suy nghĩ nên được đề cập ở cấp độ dự án, nhận %q", cfg.Thinking)
	}
	if got := cfg.Roles["writer"].Thinking; got != "low" {
		t.Errorf("role.writer.thinking nên được ghi đè ở cấp dự án, nhận %q", got)
	}
}

func TestMergeConfig_ProviderExtraFields(t *testing.T) {
	base := Config{
		Provider:  "openrouter",
		ModelName: "google/gemini-2.5-flash",
		Providers: map[string]ProviderConfig{
			"openrouter": {
				APIKey: "sk-test-123456",
				ExtraBody: map[string]any{
					"temperature": 0.8,
				},
				Extra: map[string]any{
					"user_agent": "base-client/1.0",
				},
			},
		},
	}
	overlay := Config{
		Providers: map[string]ProviderConfig{
			"openrouter": {
				BaseURL: "https://proxy.example.com/v1",
				ExtraBody: map[string]any{
					"min_p": 0.05,
				},
				Extra: map[string]any{
					"user_agent": "override-client/1.0",
					"headers": map[string]any{
						"X-Custom-Client": "ainovel",
					},
				},
			},
		},
	}

	cfg := mergeConfig(base, overlay)
	pc := cfg.Providers["openrouter"]
	if pc.APIKey != "sk-test-123456" {
		t.Fatalf("APIKey = %q, want inherited key", pc.APIKey)
	}
	if pc.BaseURL != "https://proxy.example.com/v1" {
		t.Fatalf("BaseURL = %q, want overlay URL", pc.BaseURL)
	}
	if _, ok := pc.ExtraBody["temperature"]; ok {
		t.Fatalf("ExtraBody should be replaced by overlay, got %#v", pc.ExtraBody)
	}
	if got := pc.ExtraBody["min_p"]; got != 0.05 {
		t.Fatalf("ExtraBody[min_p] = %#v, want 0.05", got)
	}
	if got := pc.Extra["user_agent"]; got != "override-client/1.0" {
		t.Fatalf("Extra[user_agent] = %#v, want override-client/1.0", got)
	}
	headers, ok := pc.Extra["headers"].(map[string]any)
	if !ok {
		t.Fatalf("Extra[headers] missing or invalid: %#v", pc.Extra["headers"])
	}
	if got := headers["X-Custom-Client"]; got != "ainovel" {
		t.Fatalf("Extra.headers[X-Custom-Client] = %#v, want ainovel", got)
	}
}

// Nguyên nhân gốc rễ 2 (vấn đề cốt lõi tái phát số 37): Nhà cung cấp ghi đè cấp dự án nhưng không khai báo thông tin xác thực tương ứng của nhà cung cấp.
// ValidateBase phải báo cáo lỗi cấu hình (thay vì gặp sự cố sâu hơn sau khi phát hành).
func TestValidateBase_ProviderOverrideWithoutCredentials(t *testing.T) {
	cfg := Config{
		Provider:  "mimo",
		ModelName: "mimo-v2.5-pro",
		Providers: map[string]ProviderConfig{
			"openrouter": {APIKey: "sk-test-123456"},
		},
	}
	cfg.FillDefaults()
	err := cfg.ValidateBase()
	if err == nil {
		t.Fatal("Nếu nhà cung cấp thiếu thông tin xác thực, lỗi sẽ được báo cáo.")
	}
	if !errors.Is(err, errs.ErrConfig) {
		t.Errorf("Nên gói errs.ErrConfig, lấy: %v", err)
	}
}

// Ví dụ tích hợp (config.example.jsonc của go:embed) phải tự nhất quán: sau khi xóa nhận xét, đó là JSON hợp pháp,
// Con trỏ của nhà cung cấp cấp cao nhất không lủng lẳng và nó phá vỡ tâm lý "con trỏ" - nó là một mẫu mà người dùng sao chép và nếu nó bị hỏng, nó sẽ đánh lừa người khác.
func TestExampleConfigIsValidAndSelfConsistent(t *testing.T) {
	if exampleConfig == "" {
		t.Fatal("go:embed không hiệu quả và exampleConfig trống")
	}
	var cfg Config
	if err := json.Unmarshal(stripJSONComments([]byte(exampleConfig)), &cfg); err != nil {
		t.Fatalf("Ví dụ tích hợp không phải là JSON hợp pháp sau khi được nhận xét (người dùng sẽ bị lừa nếu sao chép nó): %v", err)
	}
	if cfg.Provider == "" || cfg.ModelName == "" {
		t.Fatal("Các ví dụ sẽ cung cấp cho nhà cung cấp/mô hình mặc định")
	}
	if _, ok := cfg.Providers[cfg.Provider]; !ok {
		t.Errorf("Ví dụ về nhà cung cấp cấp cao nhất %q không trỏ đến mục nhập trong nhà cung cấp - mẫu phía trước con trỏ tự treo lủng lẳng", cfg.Provider)
	}
	if !contains(exampleConfig, "con trỏ") {
		t.Error("Ví dụ nên xua tan \"nhà cung cấp là con trỏ\" - đừng để bẫy nhận thức #37 quay trở lại")
	}
}

func TestWriteStartupError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := WriteStartupError("boom: provider not configured")
	if path == "" {
		t.Fatal("Đường dẫn vị trí phải được trả lại")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Đọc lỗi cuối cùng.log: %v", err)
	}
	if want := "boom: provider not configured"; !contains(string(data), want) {
		t.Errorf("Nhật ký phải chứa %q, thực tế: %s", want, data)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
