package host

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host/imp"
	"github.com/voocel/ainovel-cli/internal/store"
)

// newFlagTestHost tạo một Máy chủ tối thiểu, vừa đủ để điều khiển máy trạng thái cờ đồng tạo và các bộ bảo vệ đồng thời.
// phátEvent sử dụng recovery + non-blocking select để đệm kênh sự kiện mà không cần điều phối viên/người quan sát.
// Nhánh đang chạy của PauseForCoCreate sẽ điều chỉnh điều phối viên.Abort (sử dụng lại đường dẫn tạm dừng Esc đã được xác minh),
// Chưa được thử nghiệm ở đây; chỉ trạng thái không chạy và logic đánh dấu/bảo vệ không phụ thuộc vào bộ điều phối mới được đề cập ở đây.
func newFlagTestHost(lc lifecycle, cocreating bool) *Host {
	return &Host{
		lifecycle:  lc,
		cocreating: cocreating,
		events:     make(chan Event, 16),
	}
}

func TestPauseForCoCreate_NonRunningSetsFlag(t *testing.T) {
	h := newFlagTestHost(lifecycleIdle, false)
	if !h.PauseForCoCreate() {
		t.Fatal("Trạng thái nhàn rỗi nên được phép bước vào giai đoạn đồng sáng tạo")
	}
	if !h.cocreating {
		t.Error("cocreating phải đúng sau khi nhập")
	}
	if h.lifecycle != lifecycleIdle {
		t.Errorf("Không nên thay đổi vòng đời khi chuyển sang trạng thái không chạy và thu được %s.", h.lifecycle)
	}
}

func TestPauseForCoCreate_RejectsCompleted(t *testing.T) {
	h := newFlagTestHost(lifecycleCompleted, false)
	if h.PauseForCoCreate() {
		t.Error("Cuốn sách sau khi hoàn thành không được phép bước vào giai đoạn đồng sáng tạo.")
	}
	if h.cocreating {
		t.Error("Không nên đặt bit sau khi đồng tạo từ chối")
	}
}

func TestPauseForCoCreate_RejectsReentrant(t *testing.T) {
	h := newFlagTestHost(lifecyclePaused, true)
	if h.PauseForCoCreate() {
		t.Error("Đã trong quá trình đồng sáng tạo, việc nhập lại sẽ bị từ chối")
	}
}

func TestCancelCoCreate_ClearsFlag(t *testing.T) {
	h := newFlagTestHost(lifecyclePaused, true)
	h.CancelCoCreate()
	if h.cocreating {
		t.Error("cocreating phải được xóa sau khi hủy")
	}
	if h.lifecycle != lifecyclePaused {
		t.Errorf("Hủy bỏ không nên thay đổi vòng đời, nhận %s", h.lifecycle)
	}
}

func TestCancelCoCreate_NoopWhenNotCocreating(t *testing.T) {
	h := newFlagTestHost(lifecycleRunning, false)
	h.CancelCoCreate() // Không nên hoảng loạn, không nên thay đổi trạng thái
	if h.cocreating || h.lifecycle != lifecycleRunning {
		t.Error("Trạng thái không đồng sáng tạo CancelCoCreate sẽ không hoạt động")
	}
}

func TestResumeFromCoCreate_RejectsEmptyDraft(t *testing.T) {
	h := newFlagTestHost(lifecyclePaused, true)
	if err := h.ResumeFromCoCreate("   "); err == nil {
		t.Fatal("Một bản nháp trống sẽ báo lỗi")
	}
	if !h.cocreating {
		t.Error("Bản nháp trống được trả về trước khi xóa dấu, quá trình tạo vẫn phải đúng")
	}
}

func TestResumeFromCoCreate_RejectsWhenNotCocreating(t *testing.T) {
	h := newFlagTestHost(lifecyclePaused, false)
	err := h.ResumeFromCoCreate("## Theo dõi \n- Nhập tập 2")
	if err == nil || !strings.Contains(err.Error(), "not in co-create") {
		t.Fatalf("Trạng thái không đồng tạo sẽ được thưởng bằng trạng thái không đồng tạo và nhận %v", err)
	}
}

func TestGuardExclusive(t *testing.T) {
	cases := []struct {
		name       string
		lc         lifecycle
		cocreating bool
		wantErr    string // Trống = Mong đợi phát hành
	}{
		{"running", lifecycleRunning, false, "đang chạy"},
		{"cocreating", lifecyclePaused, true, "đồng sáng tạo theo giai đoạn"},
		{"idle free", lifecycleIdle, false, ""},
		{"paused free", lifecyclePaused, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newFlagTestHost(c.lc, c.cocreating)
			err := h.guardExclusive("nhập khẩu")
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("Nên phát hành với %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("Nên chứa %q, lấy %v", c.wantErr, err)
			}
			if !strings.Contains(err.Error(), "nhập khẩu") {
				t.Errorf("Lỗi copy nên có thao tác %q, lấy %v", "nhập khẩu", err)
			}
		})
	}
}

// TestStageCoCreate_OccupancyBlocksConcurrentEntries xác minh rằng tất cả các mục độc quyền trong cửa sổ đồng sáng tạo đều bị chặn:
// Việc nhập/bắt đầu/tiếp tục/tiếp tục phải bị từ chối trong thời gian đồng tạo và chỉ kiểm tra ==chạy trong khoảng thời gian bị tạm dừng để bù đắp khoảng trống.
func TestStageCoCreate_OccupancyBlocksConcurrentEntries(t *testing.T) {
	h := newFlagTestHost(lifecycleIdle, false)
	if !h.PauseForCoCreate() {
		t.Fatal("Lỗi đồng sáng tạo ở giai đoạn đầu")
	}

	if _, err := h.ImportFrom(context.Background(), imp.Options{}); err == nil {
		t.Error("ImportFrom trong cửa sổ đồng sáng tạo sẽ bị từ chối")
	}
	if err := h.StartPrepared("viết một câu chuyện mới"); err == nil {
		t.Error("StartPrepared trong cửa sổ đồng sáng tạo sẽ bị từ chối")
	}
	if _, err := h.Resume(); err == nil {
		t.Error("Tiếp tục trong thời gian đồng sáng tạo sẽ bị từ chối")
	}
	if err := h.Continue("tiếp tục viết"); err == nil {
		t.Error("Việc tiếp tục trong thời gian đồng sáng tạo sẽ bị từ chối")
	}

	// Nghề nghiệp được giải phóng sau khi thoát khỏi quá trình đồng sáng tạo (hủy được sử dụng ở đây; đường dẫn chèn Tiếp tục yêu cầu xác minh của người điều phối và tích hợp)
	h.CancelCoCreate()
	if h.cocreating {
		t.Fatal("Dấu hiệu chiếm chỗ nên được dỡ bỏ sau khi thoát ra")
	}
}

func TestBuildStoryStateSummary_NilStore(t *testing.T) {
	if got := buildStoryStateSummary(nil); got != "" {
		t.Errorf("cửa hàng không sẽ trả về một chuỗi trống và nhận %q", got)
	}
}

func TestBuildStoryStateSummary_Populated(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Init("Thơ của bóng tối", 100); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Progress.Load()
	p.CompletedChapters = []int{1, 2, 3}
	p.TotalWordCount = 12000
	if err := st.Progress.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := st.Outline.SaveCompass(domain.StoryCompass{
		EndingDirection: "Nhân vật chính đạt đến đỉnh cao",
		OpenThreads:     []string{"Mối thù máu thịt giữa các bậc thầy vẫn chưa được trả thù"},
		EstimatedScale:  "Dự kiến ​​4-6 tập",
	}); err != nil {
		t.Fatal(err)
	}

	got := buildStoryStateSummary(st)
	for _, want := range []string{"Thơ của bóng tối", "Đã hoàn thành chương 3", "chương tiếp theo là Chương 4", "Nhân vật chính đạt đến đỉnh cao", "Mối thù máu thịt giữa các bậc thầy vẫn chưa được trả thù", "Dự kiến ​​4-6 tập"} {
		if !strings.Contains(got, want) {
			t.Errorf("Bản tóm tắt phải chứa %q, thực tế: \n%s", want, got)
		}
	}
}
