package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAllowsFilter(t *testing.T) {
	if New("", nil).allows("repeat") != true {
		t.Error("tất cả các sự kiện nên được cho phép theo mặc định")
	}
	n := New("", []string{"run_end", "budget"})
	if !n.allows("run_end") || !n.allows("budget") {
		t.Error("Loại được liệt kê nên được phát hành")
	}
	if n.allows("repeat") {
		t.Error("Loại chưa niêm yết nên bị chặn")
	}
	var nilN *Notifier
	if nilN.allows("run_end") {
		t.Error("nil Trình thông báo sẽ chặn mọi thứ")
	}
	nilN.Send(Notification{Kind: "run_end"}) // không nên hoảng sợ
}

func TestCommandChannelEnvAndStdin(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.txt")
	jsonFile := filepath.Join(dir, "stdin.json")

	n := New(`echo "$NOTIFY_KIND|$NOTIFY_LEVEL|$NOTIFY_TITLE|$NOTIFY_BODY" > `+envFile+` && cat > `+jsonFile, nil)
	nt := Notification{Kind: "budget", Level: "warn", Title: "ainovel: ngân sách", Body: "$8,00 đã chi tiêu"}
	n.deliver(nt) // Được gọi đồng bộ để xác nhận

	env, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("lệnh không được thực thi: %v", err)
	}
	if got := strings.TrimSpace(string(env)); got != "budget|warn|ainovel: ngân sách|$8,00 đã chi tiêu" {
		t.Errorf("Truyền biến môi trường không nhất quán: %q", got)
	}

	raw, err := os.ReadFile(jsonFile)
	if err != nil {
		t.Fatalf("stdin không được thông qua: %v", err)
	}
	var decoded Notification
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("stdin JSON bất hợp pháp: %v", err)
	}
	if decoded != nt {
		t.Errorf("stdin JSON không khớp: %+v", decoded)
	}
}

func TestCommandChannelTimeoutKill(t *testing.T) {
	n := New("sleep 30", nil)
	n.timeout = 200 * time.Millisecond

	start := time.Now()
	n.deliver(Notification{Kind: "run_end"})
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Hết thời gian chờ không thành công, chặn %v", elapsed)
	}
}
