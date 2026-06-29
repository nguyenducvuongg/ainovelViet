package imp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
	"github.com/nguyenducvuongg/ainovelViet/internal/store"
	"github.com/nguyenducvuongg/ainovelViet/internal/tools"
)

// Deps chuyển ngay lập tức các phần phụ thuộc có thể cắm được mà người chạy yêu cầu để tạo điều kiện thuận lợi cho việc thử nghiệm các mô hình.
type Deps struct {
	Store      *store.Store
	CommitTool *tools.CommitChapterTool
	LLM        LLMChat // Mô hình giống nhau là đủ, nền tảng/bộ phân tích đều là suy luận có cấu trúc
	Prompts    Prompts
}

// Lời nhắc là hai đoạn chứa các từ nhắc nhở được quy trình imp sử dụng.
type Prompts struct {
	Foundation string // nền tảng đảo ngược
	Analyzer   string // Đảo ngược một chương
}

// Run thực hiện quá trình nhập hoàn chỉnh: tách → nền tảng → vòng lặp chương.
// Chạy trong goroutine riêng của nó; kênh Sự kiện bị đóng bởi chức năng này.
//
// Sự cân bằng trong thiết kế:
//   - Quá trình hoàn tất đang chặn thực thi (tác vụ dài CLI) và người gọi có trách nhiệm mở kênh nghe goroutine;
//   - Nếu bất kỳ bước nào không thành công, nó sẽ kết thúc trực tiếp và gửi sự kiện StageError;
//   - Giai đoạn chương âm thầm bỏ qua các chương đã hoàn thành (commit_chapter là bình thường, nhưng bỏ qua LLM sẽ tiết kiệm mã thông báo).
func Run(ctx context.Context, deps Deps, opts Options) (<-chan Event, error) {
	if deps.Store == nil || deps.CommitTool == nil || deps.LLM == nil {
		return nil, fmt.Errorf("deps incomplete")
	}
	if strings.TrimSpace(opts.SourcePath) == "" {
		return nil, fmt.Errorf("source path is required")
	}

	events := make(chan Event, 32)

	go func() {
		defer close(events)
		emit := func(stage Stage, current, total int, msg string, err error) {
			ev := Event{Time: time.Now(), Stage: stage, Current: current, Total: total, Message: msg, Err: err}
			select {
			case events <- ev:
			case <-ctx.Done():
			}
		}

		// ── 1. Chia ──
		emit(StageSplitting, 0, 0, "Chia thành các chương...", nil)
		chapters, err := SplitFile(opts.SourcePath)
		if err != nil {
			emit(StageError, 0, 0, "Tách không thành công", err)
			return
		}
		total := len(chapters)
		if total == 0 {
			emit(StageError, 0, 0,
				"Không xác định được chương nào: Hỗ trợ \"Chương N/Chương/Tập/Tập/Phần/Màn\" \"Tập N\" \"Mở đầu/Nêm/Phần kết/Bổ sung/Câu chuyện bên lề\""+
					"Các tiêu đề như \"Chương N / Mở đầu\" tương thích với Markdown #, khoảng trắng có chiều rộng đầy đủ, gói [ ] và mã hóa GBK."+
					"Vui lòng xác nhận rằng tập tin thực sự là một văn bản tiểu thuyết chương.",
				fmt.Errorf("no chapters matched"))
			return
		}
		emit(StageSplitting, 0, total, fmt.Sprintf("Phân đoạn đã hoàn thành: Chương %d", total), nil)

		// ── 2. Đẩy ngược nền tảng (bỏ qua nếu hoàn thành)──
		if needsFoundation(deps.Store, opts) {
			emit(StageFoundation, 0, total, "Ngược lại trong Foundation (một cuộc gọi LLM)...", nil)
			fr, err := ReverseFoundation(ctx, deps.LLM, deps.Prompts.Foundation, chapters)
			if err != nil {
				emit(StageError, 0, total, "Đẩy ngược nền tảng không thành công", err)
				return
			}
			scale := pickScale(total)
			if err := PersistFoundation(ctx, deps.Store, scale, fr); err != nil {
				emit(StageError, 0, total, "Vị trí nền tảng không thành công", err)
				return
			}
			emit(StageFoundation, 0, total,
				fmt.Sprintf("Sẵn sàng cho nền tảng: Ký tự %d / Quy tắc %d / Nội dung chương %d (Tập 1)",
					len(fr.Characters), len(fr.WorldRules), len(domain.FlattenOutline(fr.Volumes))),
				nil)
		} else {
			emit(StageFoundation, 0, total, "Nền tảng đã tồn tại, bỏ qua việc đẩy lùi", nil)
		}

		// ── 3. Chu kỳ chương ──
		premise, _ := deps.Store.Outline.LoadPremise()
		charactersBlock := loadCharactersBlock(deps.Store)

		startIdx := 0
		if opts.ResumeFrom > 1 {
			startIdx = opts.ResumeFrom - 1
		}
		for i := startIdx; i < total; i++ {
			if err := ctx.Err(); err != nil {
				emit(StageError, i+1, total, "Người dùng hủy", err)
				return
			}
			chNum := i + 1
			ch := chapters[i]

			// Đã hoàn thành → Bỏ qua LLM
			if deps.Store.Progress.IsChapterCompleted(chNum) {
				emit(StageChapter, chNum, total, fmt.Sprintf("Chương %d đã hoàn thành, bị bỏ qua", chNum), nil)
				continue
			}

			emit(StageChapter, chNum, total, fmt.Sprintf("Chương phân tích %d/%d: %s", chNum, total, ch.Title), nil)

			activeHooks, _ := deps.Store.World.LoadActiveForeshadow()
			analysis, err := AnalyzeChapter(ctx, deps.LLM, deps.Prompts.Analyzer,
				chNum, ch.Title, ch.Content, premise, charactersBlock, activeHooks)
			if err != nil {
				emit(StageError, chNum, total, fmt.Sprintf("Chương Phân tích %d không thành công", chNum), err)
				return
			}

			if err := PersistChapter(ctx, deps.Store, deps.CommitTool, chNum, ch.Title, ch.Content, analysis); err != nil {
				emit(StageError, chNum, total, fmt.Sprintf("Chương %d Vị trí không thành công", chNum), err)
				return
			}
			emit(StageChapter, chNum, total, fmt.Sprintf("Chương %d đã hoàn tất nhập", chNum), nil)
		}

		emit(StageDone, total, total, fmt.Sprintf("Đã nhập xong: Chương %d", total), nil)
	}()

	return events, nil
}

// NeedFoundation xác định liệu nền tảng có cần được đẩy lùi lại hay không.
// Nếu người dùng rõ ràng ResumeFrom > 1, nó sẽ được coi là "nhập tiếp theo" và việc đẩy ngược sẽ bị bỏ qua; nếu không, nó sẽ được đánh giá theo trạng thái Cửa hàng.
func needsFoundation(st *store.Store, opts Options) bool {
	if opts.ResumeFrom > 1 {
		return false
	}
	return len(st.FoundationMissing()) > 0
}

// pickScale đưa ra giá trị ban đầu hợp lý cho cấp độ lập kế hoạch dựa trên số chương; ngắn 25, giữa 80, nếu không thì dài.
// Nó không ảnh hưởng đến việc nhập chính nó mà chỉ ảnh hưởng đến việc lựa chọn từ nhắc kiến ​​trúc sư của Điều phối viên khi tiếp tục viết.
func pickScale(total int) domain.PlanningTier {
	switch {
	case total <= 25:
		return domain.PlanningTierShort
	case total <= 80:
		return domain.PlanningTierMid
	default:
		return domain.PlanningTierLong
	}
}

// LoadCharactersBlock hiển thị hồ sơ nhân vật thành một khối văn bản ngắn (tên/vai trò + một mô tả câu),
// Chỉ để tham khảo trong ngữ cảnh LLM, không yêu cầu cấu trúc nghiêm ngặt.
func loadCharactersBlock(st *store.Store) string {
	chars, err := st.Characters.Load()
	if err != nil || len(chars) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, c := range chars {
		fmt.Fprintf(&sb, "- **%s**（%s）：%s\n", c.Name, c.Role, oneLine(c.Description))
	}
	return sb.String()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
