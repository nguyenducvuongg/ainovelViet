package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Thiết lập khởi tạo trình ghi nhật ký mặc định của nhật ký.
// w là mục tiêu đầu ra của nhật ký và cấp độ là mức nhật ký thấp nhất.
func Setup(w io.Writer, level slog.Level) {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Thời gian chỉ giữ lại giờ, phút, giây để tiết kiệm diện tích
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("15:04:05"))
			}
			return a
		},
	})
	slog.SetDefault(slog.New(h))
}

// SetupFile khởi tạo nhật ký thành một tệp và trả về chức năng dọn dẹp.
// Khi cũngStderr=true, xuất ra stderr cùng lúc.
func SetupFile(outputDir, filename string, alsoStderr bool) func() {
	logPath := filepath.Join(outputDir, "logs", filename)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		Setup(io.Discard, slog.LevelInfo)
		return func() {}
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		Setup(io.Discard, slog.LevelInfo)
		return func() {}
	}

	var w io.Writer = f
	if alsoStderr {
		w = io.MultiWriter(os.Stderr, f)
	}
	Setup(w, slog.LevelDebug)

	return func() { _ = f.Close() }
}
