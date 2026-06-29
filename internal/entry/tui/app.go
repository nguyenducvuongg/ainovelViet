package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/logger"
)

// Chạy khởi động TUI.
// Quy ước phân lớp chế độ khởi động:
// 1. Chế độ nhanh và chế độ đồng sáng tạo thuộc về "điều phối khởi động";
// 2. Phiên tạo chính thức vào Host.Host;
// 3. Nếu các chế độ chia sẻ mới như "tiếp tục tiểu thuyết hiện có" được thêm vào trong tương lai, chúng sẽ được hợp nhất thành nội bộ/mục nhập/khởi động.
func Run(cfg bootstrap.Config, bundle assets.Bundle, version string) error {
	rt, err := host.New(cfg, bundle)
	if err != nil {
		return err
	}
	bridge := newAskUserBridge()
	rt.AskUser().SetHandler(bridge.handler)
	cleanup := logger.SetupFile(rt.Dir(), "tui.log", false)
	defer cleanup()
	defer rt.Close()

	m := NewModel(rt, bridge, version)
	// Không bật báo cáo chuột trên toàn cầu khi khởi động: chuột không được sử dụng trên trang chào mừng. Tắt báo cáo có thể giữ lại thiết bị đầu cuối gốc
	// Kéo để chọn và sao chép. Khi vào bàn làm việc sáng tạo (chế độ Đang chạy), hãy sử dụng enterRunning để mở báo cáo.
	// Để hỗ trợ nhấp vào bảng cắt/bánh xe/thanh bên kéo.
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
