package store

import (
	"testing"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
)

func newCastTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestCastMergeAppearances_NewEntries(t *testing.T) {
	s := newCastTestStore(t)
	intros := []domain.CastIntro{{Name: "Lão Châu", BriefRole: "chủ quán trọ"}}
	if err := s.Cast.MergeAppearances(5, []string{"Lão Châu", "Ayun"}, intros, nil); err != nil {
		t.Fatalf("MergeAppearances: %v", err)
	}

	entries, err := s.Cast.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.FirstSeenChapter != 5 || e.LastSeenChapter != 5 || e.AppearanceCount != 1 {
			t.Errorf("entry %s: unexpected appearance fields %+v", e.Name, e)
		}
		if e.Name == "Lão Châu" && e.BriefRole != "chủ quán trọ" {
			t.Errorf("mong đợi BriefRole chủ quán trọ cho Lão Chu, đã nhận %q", e.BriefRole)
		}
		if e.Name == "Ayun" && e.BriefRole != "" {
			t.Errorf("Ayun không có phần giới thiệu, BriefRole sẽ trống và nhận %q", e.BriefRole)
		}
	}
}

func TestCastMergeAppearances_AccumulatesOnRepeat(t *testing.T) {
	s := newCastTestStore(t)
	if err := s.Cast.MergeAppearances(5, []string{"Lão Châu"}, nil, nil); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	if err := s.Cast.MergeAppearances(8, []string{"Lão Châu"}, nil, nil); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	entries, _ := s.Cast.Load()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.FirstSeenChapter != 5 || e.LastSeenChapter != 8 || e.AppearanceCount != 2 {
		t.Fatalf("expected first=5,last=8,count=2; got %+v", e)
	}
	if len(e.AppearanceChapters) != 2 || e.AppearanceChapters[0] != 5 || e.AppearanceChapters[1] != 8 {
		t.Errorf("AppearanceChapters wrong: %v", e.AppearanceChapters)
	}
}

func TestCastMergeAppearances_IsIdempotent(t *testing.T) {
	s := newCastTestStore(t)
	if err := s.Cast.MergeAppearances(5, []string{"Lão Châu"}, nil, nil); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	// Cam kết chương tương tự được kích hoạt nhiều lần (kịch bản khôi phục sự cố hoặc viết lại)
	if err := s.Cast.MergeAppearances(5, []string{"Lão Châu"}, nil, nil); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	entries, _ := s.Cast.Load()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].AppearanceCount != 1 {
		t.Errorf("expected AppearanceCount=1 after duplicate, got %d", entries[0].AppearanceCount)
	}
}

func TestCastMergeAppearances_FiltersCoreCharacters(t *testing.T) {
	s := newCastTestStore(t)
	core := map[string]bool{"Lâm Mạch": true, "Lý Thanh Nham": true}
	if err := s.Cast.MergeAppearances(3, []string{"Lâm Mạch", "Lý Thanh Nham", "Lão Châu"}, nil, core); err != nil {
		t.Fatalf("MergeAppearances: %v", err)
	}

	entries, _ := s.Cast.Load()
	if len(entries) != 1 || entries[0].Name != "Lão Châu" {
		t.Fatalf("dự kiến ​​chỉ có lao zhou trong sổ cái, có %+v", entries)
	}
}

func TestCastMergeAppearances_BackfillsBriefRole(t *testing.T) {
	s := newCastTestStore(t)
	// Chương 5 giới thiệu Lão Chu nhưng tác giả quên điền tóm tắt
	if err := s.Cast.MergeAppearances(5, []string{"Lão Châu"}, nil, nil); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	// Chương 8 lại xuất hiện, lần này Tác giả bổ sung thêm tóm tắt_role
	intros := []domain.CastIntro{{Name: "Lão Châu", BriefRole: "chủ quán trọ"}}
	if err := s.Cast.MergeAppearances(8, []string{"Lão Châu"}, intros, nil); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	entries, _ := s.Cast.Load()
	if entries[0].BriefRole != "chủ quán trọ" {
		t.Errorf("mong đợi BriefRole chủ nhà trọ đã được hỗ trợ, đã nhận %q", entries[0].BriefRole)
	}
}

func TestCastMergeAppearances_NoOverwriteBriefRole(t *testing.T) {
	s := newCastTestStore(t)
	// Chương 5 Xác định BriefRole=Innkeeper
	if err := s.Cast.MergeAppearances(5,
		[]string{"Lão Châu"},
		[]domain.CastIntro{{Name: "Lão Châu", BriefRole: "chủ quán trọ"}},
		nil,
	); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	// Chương 8 Người viết đã chuyển nhầm một BriefRole khác (không nên ghi đè)
	if err := s.Cast.MergeAppearances(8,
		[]string{"Lão Châu"},
		[]domain.CastIntro{{Name: "Lão Châu", BriefRole: "Người đánh bạc"}},
		nil,
	); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	entries, _ := s.Cast.Load()
	if entries[0].BriefRole != "chủ quán trọ" {
		t.Errorf("expected BriefRole NOT overwritten, got %q", entries[0].BriefRole)
	}
}

func TestCastRecentActive_OrdersByLastSeen(t *testing.T) {
	s := newCastTestStore(t)
	_ = s.Cast.MergeAppearances(3, []string{"A"}, nil, nil)
	_ = s.Cast.MergeAppearances(10, []string{"B"}, nil, nil)
	_ = s.Cast.MergeAppearances(7, []string{"C"}, nil, nil)

	recent, err := s.Cast.RecentActive(2)
	if err != nil {
		t.Fatalf("RecentActive: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2, got %d", len(recent))
	}
	if recent[0].Name != "B" || recent[1].Name != "C" {
		t.Errorf("expected order B, C; got %s, %s", recent[0].Name, recent[1].Name)
	}
}

func TestCastRecentActive_SkipsPromoted(t *testing.T) {
	s := newCastTestStore(t)
	if err := s.Cast.Save([]domain.CastEntry{
		{Name: "Lõi nâng cấp", LastSeenChapter: 20, AppearanceCount: 8, Promoted: true},
		{Name: "Vai trò hỗ trợ tích cực", LastSeenChapter: 18, AppearanceCount: 3},
		{Name: "một vai phụ khác", LastSeenChapter: 15, AppearanceCount: 2},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	recent, err := s.Cast.RecentActive(10)
	if err != nil {
		t.Fatalf("RecentActive: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 (Promoted excluded), got %d: %+v", len(recent), recent)
	}
	for _, e := range recent {
		if e.Promoted {
			t.Errorf("Promoted entry leaked into RecentActive: %+v", e)
		}
	}
	if recent[0].Name != "Vai trò hỗ trợ tích cực" {
		t.Errorf("được mong đợi đầu tiên=vai trò hỗ trợ tích cực, có %s", recent[0].Name)
	}
}

func TestCastMergeAppearances_NoOpOnEmpty(t *testing.T) {
	s := newCastTestStore(t)
	if err := s.Cast.MergeAppearances(5, nil, nil, nil); err != nil {
		t.Fatalf("MergeAppearances empty: %v", err)
	}
	if err := s.Cast.MergeAppearances(0, []string{"Lão Châu"}, nil, nil); err != nil {
		t.Fatalf("MergeAppearances chapter=0: %v", err)
	}
	entries, _ := s.Cast.Load()
	if len(entries) != 0 {
		t.Errorf("expected empty ledger, got %d entries", len(entries))
	}
}
