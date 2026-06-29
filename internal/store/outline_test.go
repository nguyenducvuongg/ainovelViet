package store

import (
	"testing"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
)

func setupLayered(t *testing.T, volumes []domain.VolumeOutline) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Outline.SaveLayeredOutline(volumes); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}
	if err := s.Progress.SetLayered(true); err != nil {
		t.Fatalf("SetLayered: %v", err)
	}
	return s
}

func TestCheckArcBoundaryNeedsNewVolume(t *testing.T) {
	// Chỉ có 1 tập, 1 arc và 1 chương, chưa phải Final → NeedsNewVolume nên kích hoạt
	s := setupLayered(t, []domain.VolumeOutline{{
		Index: 1, Title: "Tập 1", Theme: "Bắt đầu",
		Arcs: []domain.ArcOutline{{
			Index: 1, Title: "cung đầu tiên", Goal: "Mục tiêu",
			Chapters: []domain.OutlineEntry{{Title: "Chương 1", CoreEvent: "khai mạc", Hook: "Tiếp tục"}},
		}},
	}})

	b, err := s.Outline.CheckArcBoundary(1) // Chương 1 = chương cuối cùng/tập
	if err != nil {
		t.Fatalf("CheckArcBoundary: %v", err)
	}
	if b == nil {
		t.Fatal("expected boundary, got nil")
	}
	if !b.IsArcEnd || !b.IsVolumeEnd {
		t.Fatalf("expected arc+volume end, got arc=%v vol=%v", b.IsArcEnd, b.IsVolumeEnd)
	}
	if !b.NeedsNewVolume {
		t.Fatal("expected NeedsNewVolume=true")
	}
	if b.NextVolume != 0 || b.NextArc != 0 {
		t.Fatalf("expected no next, got vol=%d arc=%d", b.NextVolume, b.NextArc)
	}
}

func TestCheckArcBoundaryLastVolumeRequiresDecision(t *testing.T) {
	// Chương cuối của tập đơn → kích hoạt NeedsNewVolume và để Router cho kiến ​​trúc sư chọn một trong hai:
	// append_volume tiếp tục viết / Complete_book kết thúc.
	s := setupLayered(t, []domain.VolumeOutline{{
		Index: 1, Title: "chỉ âm lượng", Theme: "chủ đề",
		Arcs: []domain.ArcOutline{{
			Index: 1, Title: "vòng cung duy nhất", Goal: "đóng gói",
			Chapters: []domain.OutlineEntry{{Title: "Chương cuối cùng", CoreEvent: "kết thúc", Hook: "không có"}},
		}},
	}})

	b, err := s.Outline.CheckArcBoundary(1)
	if err != nil {
		t.Fatalf("CheckArcBoundary: %v", err)
	}
	if !b.NeedsNewVolume {
		t.Fatal("expected NeedsNewVolume=true at last expanded chapter")
	}
	if b.HasNextArc() {
		t.Fatal("expected no next arc")
	}
}

func TestCheckArcBoundaryNextArcInSameVolume(t *testing.T) {
	// 2 cung: Phần cuối của cung thứ 1 sẽ trỏ đến cung thứ 2 và NeedsNewVolume sẽ không được kích hoạt.
	s := setupLayered(t, []domain.VolumeOutline{{
		Index: 1, Title: "Tập 1", Theme: "Bắt đầu",
		Arcs: []domain.ArcOutline{
			{Index: 1, Title: "cung đầu tiên", Goal: "Mục tiêu", Chapters: []domain.OutlineEntry{{Title: "Chương 1", CoreEvent: "sự kiện", Hook: "cái móc"}}},
			{Index: 2, Title: "cung thứ cấp", Goal: "Mục tiêu 2", EstimatedChapters: 10},
		},
	}})

	b, err := s.Outline.CheckArcBoundary(1)
	if err != nil {
		t.Fatalf("CheckArcBoundary: %v", err)
	}
	if !b.IsArcEnd {
		t.Fatal("expected arc end")
	}
	if b.IsVolumeEnd {
		t.Fatal("expected not volume end (second arc exists)")
	}
	if b.NeedsNewVolume {
		t.Fatal("expected NeedsNewVolume=false")
	}
	if b.NextVolume != 1 || b.NextArc != 2 {
		t.Fatalf("expected next vol=1 arc=2, got vol=%d arc=%d", b.NextVolume, b.NextArc)
	}
	if !b.NeedsExpansion {
		t.Fatal("expected NeedsExpansion=true for skeleton arc")
	}
}

func TestAppendVolumeValidation(t *testing.T) {
	s := setupLayered(t, []domain.VolumeOutline{{
		Index: 1, Title: "Tập 1", Theme: "Bắt đầu",
		Arcs: []domain.ArcOutline{{
			Index: 1, Title: "cung đầu tiên", Goal: "Mục tiêu",
			Chapters: []domain.OutlineEntry{{Title: "chương", CoreEvent: "sự kiện", Hook: "cái móc"}},
		}},
	}})

	validVol := domain.VolumeOutline{
		Index: 2, Title: "Tập 2", Theme: "nâng cấp",
		Arcs: []domain.ArcOutline{{
			Index: 1, Title: "cung một", Goal: "Mục tiêu",
			Chapters: []domain.OutlineEntry{{Title: "chương mới", CoreEvent: "nâng cao", Hook: "cái móc"}},
		}},
	}

	// Nối thêm bình thường sẽ thành công
	if err := s.AppendVolume(validVol); err != nil {
		t.Fatalf("AppendVolume valid: %v", err)
	}

	// Chỉ số không tăng → thất bại
	if err := s.AppendVolume(domain.VolumeOutline{
		Index: 1, Title: "lặp lại", Theme: "x",
		Arcs: []domain.ArcOutline{{Index: 1, Title: "vòng cung", Goal: "g", Chapters: []domain.OutlineEntry{{Title: "ch", CoreEvent: "e", Hook: "h"}}}},
	}); err == nil {
		t.Fatal("expected error for non-increasing index")
	}

	// Không có vòng cung → thất bại
	if err := s.AppendVolume(domain.VolumeOutline{Index: 3, Title: "vô giá trị", Theme: "x"}); err == nil {
		t.Fatal("expected error for volume with no arcs")
	}

	// Phần đầu tiên không có chương → thất bại
	if err := s.AppendVolume(domain.VolumeOutline{
		Index: 3, Title: "bộ xương", Theme: "x",
		Arcs: []domain.ArcOutline{{Index: 1, Title: "vòng cung", Goal: "g", EstimatedChapters: 10}},
	}); err == nil {
		t.Fatal("expected error for first arc without chapters")
	}
}

// Lưu ý: Ngữ nghĩa ban đầu của việc từ chối nối thêm bằng cách sử dụng tập Cuối cùng đã được hạ xuống lớp save_foundation (Phase=Hoàn thành từ chối).
// Xem save_foundation_test.go::TestSaveFoundationAppendVolumeRejectsAfterComplete.
// Lớp lưu trữ chỉ giữ lại xác minh cấu trúc (Gia tăng chỉ mục/cung đầu tiên chứa các chương, v.v.).

func TestSaveAndLoadCompass(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Hướng trống sẽ thất bại
	if err := s.Outline.SaveCompass(domain.StoryCompass{EstimatedScale: "3 tập"}); err == nil {
		t.Fatal("expected error for empty ending_direction")
	}

	// Lưu bình thường
	compass := domain.StoryCompass{
		EndingDirection: "Nhân vật chính phải đối mặt với sự lựa chọn cuối cùng",
		OpenThreads:     []string{"Đầu mối A", "Mối quan hệ B"},
		EstimatedScale:  "Dự kiến ​​4-6 tập",
		LastUpdated:     12,
	}
	if err := s.Outline.SaveCompass(compass); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}

	loaded, err := s.Outline.LoadCompass()
	if err != nil {
		t.Fatalf("LoadCompass: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected compass, got nil")
	}
	if loaded.EndingDirection != "Nhân vật chính phải đối mặt với sự lựa chọn cuối cùng" {
		t.Fatalf("expected direction %q, got %q", "Nhân vật chính phải đối mặt với sự lựa chọn cuối cùng", loaded.EndingDirection)
	}
	if len(loaded.OpenThreads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(loaded.OpenThreads))
	}
}
