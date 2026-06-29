package store

import (
	"os"
	"slices"
	"sort"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
)

// CastStore quản lý danh sách diễn viên phụ (meta/cast_ledger.json).
//
// Danh sách diễn viên phụ ghi lại "các nhân vật phụ được đặt tên đã xuất hiện", trực giao với character.json (tệp nhân vật cốt lõi):
//   - character.json: Nhân vật chính + các vai phụ chính được Architect thiết kế rõ ràng, sẽ không được sửa đổi trong suốt thời gian viết
//   - cast_ledger.json: Công cụ commit_chapter tự động cộng dồn tất cả các vai trò hỗ trợ không cốt lõi kèm theo tên
//
// MergeAppearances là bình thường: các cam kết lặp lại của cùng một chương sẽ không tích lũy AppearanceCount nhiều lần.
type CastStore struct{ io *IO }

func NewCastStore(io *IO) *CastStore { return &CastStore{io: io} }

const castLedgerPath = "meta/cast_ledger.json"

// Load đọc danh sách diễn viên phụ. Trả về một lát trống nếu tệp không tồn tại.
func (s *CastStore) Load() ([]domain.CastEntry, error) {
	var entries []domain.CastEntry
	if err := s.io.ReadJSON(castLedgerPath, &entries); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return entries, nil
}

// Lưu Lưu toàn bộ danh sách diễn viên phụ (viết nguyên tử).
func (s *CastStore) Save(entries []domain.CastEntry) error {
	return s.io.WriteJSON(castLedgerPath, entries)
}

// MergeAppearances Hợp nhất các bản ghi ngoại hình của chương này vào danh sách.
//
// tham số:
//   - chương: số chương
//   - ký tự: mảng tên xuất hiện trong chương này (từ commit_chapter.Characters)
//   - phần giới thiệu: Phần giới thiệu vai trò mới được tuyên bố rõ ràng của Người viết (lần đầu xuất hiện hoặc hoàn thành BriefRole)
//   - knownCore: Một tập hợp các tên nhân vật cốt lõi đã có trong character.json (những tên này bỏ qua việc ghi vào sổ cái)
//
// Hành vi:
//   - Tên trong knownCore: bị bỏ qua (tệp ký tự lõi là mục nhập bản ghi duy nhất của nó)
//   - tên đã có trong sổ cái và chương đã có trong AppearanceChapters: bị bỏ qua hoàn toàn (idempotent)
//   - tên đã có trong sổ cái nhưng chương mới: cập nhật LastSeenChapter + nối thêm chương + đếm++
//   - Không có tên trong sổ cái: thêm mục mới
//   - BriefRole trong phần giới thiệu chỉ được lấy nếu mục sổ cái BriefRole vẫn trống, để tránh ghi đè phần giới thiệu trước đó
func (s *CastStore) MergeAppearances(
	chapter int,
	characters []string,
	intros []domain.CastIntro,
	knownCore map[string]bool,
) error {
	if chapter <= 0 || len(characters) == 0 {
		return nil
	}
	return s.io.WithWriteLock(func() error {
		var entries []domain.CastEntry
		if err := s.io.ReadJSONUnlocked(castLedgerPath, &entries); err != nil && !os.IsNotExist(err) {
			return err
		}

		introMap := make(map[string]string, len(intros))
		for _, in := range intros {
			if in.Name != "" {
				introMap[in.Name] = in.BriefRole
			}
		}

		index := make(map[string]int, len(entries))
		for i, e := range entries {
			index[e.Name] = i
			for _, alias := range e.Aliases {
				index[alias] = i
			}
		}

		seen := make(map[string]bool, len(characters))
		for _, name := range characters {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			if knownCore[name] {
				continue
			}
			if i, ok := index[name]; ok {
				entry := &entries[i]
				if !slices.Contains(entry.AppearanceChapters, chapter) {
					entry.AppearanceChapters = append(entry.AppearanceChapters, chapter)
					entry.AppearanceCount = len(entry.AppearanceChapters)
					if chapter > entry.LastSeenChapter {
						entry.LastSeenChapter = chapter
					}
					if chapter < entry.FirstSeenChapter || entry.FirstSeenChapter == 0 {
						entry.FirstSeenChapter = chapter
					}
				}
				if entry.BriefRole == "" {
					if br, ok := introMap[name]; ok && br != "" {
						entry.BriefRole = br
					}
				}
				continue
			}
			entries = append(entries, domain.CastEntry{
				Name:               name,
				BriefRole:          introMap[name],
				FirstSeenChapter:   chapter,
				LastSeenChapter:    chapter,
				AppearanceCount:    1,
				AppearanceChapters: []int{chapter},
			})
		}
		return s.io.WriteJSONUnlocked(castLedgerPath, entries)
	})
}

// Hoạt động gần đây Trả về N mục nhập ký tự hỗ trợ hoạt động gần đây nhất (theo thứ tự ngược lại của LastSeenChapter).
// Được sử dụng trong tiểu thuyết_context để nhớ lại "các nhân vật phụ gần đây" mà Người viết có thể cần viết chương tiếp theo.
//
// Các mục nhập đã được thăng cấp lên character.json (Được thăng cấp=true) sẽ bị bỏ qua để tránh bị thu hồi nhiều lần với tệp lõi.
func (s *CastStore) RecentActive(limit int) ([]domain.CastEntry, error) {
	if limit <= 0 {
		return nil, nil
	}
	entries, err := s.Load()
	if err != nil {
		return nil, err
	}
	active := entries[:0:0]
	for _, e := range entries {
		if e.Promoted {
			continue
		}
		active = append(active, e)
	}
	if len(active) == 0 {
		return nil, nil
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].LastSeenChapter != active[j].LastSeenChapter {
			return active[i].LastSeenChapter > active[j].LastSeenChapter
		}
		return active[i].AppearanceCount > active[j].AppearanceCount
	})
	if len(active) > limit {
		active = active[:limit]
	}
	return active, nil
}
