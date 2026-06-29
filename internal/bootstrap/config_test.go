package bootstrap

import "testing"

func TestConfigResolveThinking(t *testing.T) {
	cfg := Config{
		Thinking: "low", // Mặc định cấp cao nhất
		Roles: map[string]RoleConfig{
			"writer":    {Provider: "p", Model: "m", Thinking: "high"}, // Bảo hiểm vai trò
			"architect": {Provider: "p", Model: "m"},                   // Không cần suy nghĩ, nên để mặc định
		},
	}

	cases := []struct {
		role string
		want string
	}{
		{"writer", "high"},     // Bảo hiểm vai trò được ưu tiên
		{"architect", "low"},   // Vai trò không được chỉ định → quay lại mặc định cấp cao nhất
		{"editor", "low"},      // Vai trò không tồn tại → mặc định cấp cao nhất
		{"", "low"},            // trống → mặc định cấp cao nhất
		{"default", "low"},     // mặc định → mặc định cấp cao nhất
		{"coordinator", "low"}, // Chưa được định cấu hình → mặc định cấp cao nhất
	}
	for _, c := range cases {
		if got := cfg.ResolveThinking(c.role); got != c.want {
			t.Errorf("ResolveThinking(%q) = %q, want %q", c.role, got, c.want)
		}
	}

	// Khi cấp cao nhất cũng trống theo mặc định, "" (không bị ghi đè) sẽ được trả về nếu vai trò không được bao gồm.
	empty := Config{Roles: map[string]RoleConfig{"writer": {Thinking: "xhigh"}}}
	if got := empty.ResolveThinking("editor"); got != "" {
		t.Errorf("Theo mặc định, trình chỉnh sửa sẽ trả về \"\" và nhận %q theo mặc định.", got)
	}
	if got := empty.ResolveThinking("writer"); got != "xhigh" {
		t.Errorf("Ghi đè trình ghi mặc định trống sẽ có hiệu lực, nhận %q", got)
	}
}
