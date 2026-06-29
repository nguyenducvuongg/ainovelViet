package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/entry/headless"
	"github.com/voocel/ainovel-cli/internal/entry/tui"
	"github.com/voocel/ainovel-cli/internal/rules"
	buildversion "github.com/voocel/ainovel-cli/internal/version"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// headlessMode ghi lại xem headless có được khởi động lần này hay không, do đó khuôn có thể quyết định có nên tạm dừng khi thoát khi gặp lỗi hay không.
var headlessMode bool

func main() {
	opts, args, err := parseCLIOptions(os.Args[1:])
	if err != nil {
		die("flags: %v", err)
	}
	if opts.Version {
		buildversion.Print(os.Stdout, versionInfo())
		return
	}
	if opts.Update {
		if err := runSelfUpdate(opts.UpdateVersion); err != nil {
			fmt.Fprintf(os.Stderr, "update: %v\n", err)
			os.Exit(1)
		}
		return
	}
	headlessMode = opts.Headless

	// lần khởi động đầu tiên
	if bootstrap.NeedsSetup(opts.ConfigPath) {
		if opts.Headless {
			die("lỗi: chế độ không đầu không hỗ trợ khởi động lần đầu. Vui lòng chạy TUI trước để hoàn tất cấu hình.")
		}
		setupCfg, err := bootstrap.RunSetup()
		if err != nil {
			die("setup: %v", err)
		}
		// Sau khi khởi động xong, tiếp tục sử dụng cấu hình đã tạo
		runWithConfig(setupCfg, opts, args)
		return
	}

	// Tải cấu hình
	cfg, err := bootstrap.LoadConfig(opts.ConfigPath)
	if err != nil {
		die("config: %v", err)
	}

	runWithConfig(cfg, opts, args)
}

// die xử lý thống nhất các lỗi thoát nghiêm trọng: in ra stderr, thả xuống ~/.ainovel/last-error.log,
// Và tạm dừng trong thiết bị đầu cuối tương tác (không có đầu) và đợi trả lại dòng - bảng điều khiển sẽ thoát cùng với quy trình khi bạn nhấp đúp để bắt đầu.
// Đóng ngay lập tức. Nếu bạn không tạm dừng, lỗi sẽ xuất hiện. Đây là nguyên nhân cốt lõi của sự cố số 37 mà người dùng không thể khắc phục được.
func die(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, msg)
	if path := bootstrap.WriteStartupError(msg); path != "" {
		fmt.Fprintf(os.Stderr, "(Lỗi chi tiết ghi vào %s) \n", path)
	}
	if !headlessMode && stdinIsTerminal() {
		fmt.Fprint(os.Stderr, "\n Nhấn Enter để thoát...")
		fmt.Fscanln(os.Stdin)
	}
	os.Exit(1)
}

// stdinIsTerminal xác định xem đầu vào tiêu chuẩn có được kết nối với thiết bị đầu cuối (thiết bị ký tự) hay không. Nhấp đúp để bắt đầu/thiết bị đầu cuối tương tác
// là đúng; đường ống, chuyển hướng, CI là sai. Xấp xỉ không phụ thuộc là đủ để phân biệt có nên tạm dừng hay không.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func runWithConfig(cfg bootstrap.Config, opts cliOptions, args []string) {
	rules.EnsureHomeRulesDir()

	if len(args) > 0 {
		die("lỗi: Việc chuyển trực tiếp các yêu cầu mới từ dòng lệnh không còn được hỗ trợ. Vui lòng nhập nó vào hộp nhập TUI sau khi khởi động.")
	}

	bundle := assets.Load(cfg.Style)
	if opts.Headless {
		prompt, err := loadPrompt(opts)
		if err != nil {
			die("error: %v", err)
		}
		if err := headless.Run(cfg, bundle, headless.Options{Prompt: prompt}); err != nil {
			die("error: %v", err)
		}
		return
	}
	if opts.Prompt != "" || opts.PromptFile != "" {
		die("lỗi: --prompt/--prompt-file chỉ có thể được sử dụng ở chế độ --headless")
	}
	if err := tui.Run(cfg, bundle, versionInfo().Version); err != nil {
		die("error: %v", err)
	}
}

type cliOptions struct {
	ConfigPath    string
	Headless      bool
	Prompt        string
	PromptFile    string
	Version       bool
	Update        bool
	UpdateVersion string
}

// ParsCLIOptions trích xuất cờ CLI, trả về các tùy chọn và các tham số còn lại.
func parseCLIOptions(argv []string) (cliOptions, []string, error) {
	var opts cliOptions
	var args []string
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--version", "-v":
			opts.Version = true
		case "version":
			if i+1 < len(argv) {
				return opts, nil, fmt.Errorf("phiên bản không chấp nhận tham số")
			}
			opts.Version = true
		case "update":
			if opts.Update {
				return opts, nil, fmt.Errorf("cập nhật chỉ có thể được chỉ định một lần")
			}
			opts.Update = true
			if i+1 < len(argv) {
				if strings.HasPrefix(argv[i+1], "-") {
					return opts, nil, fmt.Errorf("cập nhật chỉ chấp nhận tham số phiên bản tùy chọn")
				}
				opts.UpdateVersion = argv[i+1]
				i++
			}
			if i+1 < len(argv) {
				return opts, nil, fmt.Errorf("cập nhật chỉ chấp nhận tham số phiên bản tùy chọn")
			}
		case "--config":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--config thiếu giá trị")
			}
			opts.ConfigPath = argv[i+1]
			i++
		case "--headless":
			opts.Headless = true
		case "--prompt":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--giá trị bị thiếu nhắc nhở")
			}
			opts.Prompt = argv[i+1]
			i++
		case "--prompt-file":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--prompt-file thiếu giá trị")
			}
			opts.PromptFile = argv[i+1]
			i++
		default:
			args = append(args, argv[i])
		}
	}
	if opts.Prompt != "" && opts.PromptFile != "" {
		return opts, nil, fmt.Errorf("--prompt và --prompt-file không thể được sử dụng cùng lúc")
	}
	if opts.Version && (opts.Update || opts.ConfigPath != "" || opts.Headless || opts.Prompt != "" || opts.PromptFile != "" || len(args) > 0) {
		return opts, nil, fmt.Errorf("phiên bản không thể trộn lẫn với các tham số khởi động khác")
	}
	if opts.Update && (opts.ConfigPath != "" || opts.Headless || opts.Prompt != "" || opts.PromptFile != "" || len(args) > 0) {
		return opts, nil, fmt.Errorf("cập nhật không thể trộn lẫn với các tham số khởi động khác")
	}
	return opts, args, nil
}

func versionInfo() buildversion.Info {
	return buildversion.Resolve(buildversion.Info{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
}

func runSelfUpdate(target string) error {
	info := versionInfo()
	result, err := buildversion.Update(context.Background(), buildversion.UpdateOptions{
		Repo:           "voocel/ainovel-cli",
		BinaryName:     "ainovel-cli",
		TargetVersion:  target,
		CurrentVersion: info.Version,
	})
	if err != nil {
		return err
	}
	if !result.Updated {
		fmt.Printf("ainovel-cli là phiên bản mới nhất %s\n", result.Version)
		return nil
	}
	fmt.Printf("ainovel-cli đã được cập nhật lên %s\n", result.Version)
	fmt.Printf("Vị trí lắp đặt: %s\n", result.Path)
	return nil
}

func loadPrompt(opts cliOptions) (string, error) {
	if opts.PromptFile == "" {
		return strings.TrimSpace(opts.Prompt), nil
	}

	var data []byte
	var err error
	if opts.PromptFile == "-" {
		data, err = os.ReadFile("/dev/stdin")
	} else {
		data, err = os.ReadFile(opts.PromptFile)
	}
	if err != nil {
		return "", fmt.Errorf("Không đọc được lời nhắc: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
