package store

import (
	"fmt"
	"os"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// SummaryStore quản lý các bản tóm tắt chương, phần và tập.
type SummaryStore struct {
	io      *IO
	outline *OutlineStore // Phần phụ thuộc chỉ đọc, được sử dụng để lấy số cung/tập
}

func NewSummaryStore(io *IO, outline *OutlineStore) *SummaryStore {
	return &SummaryStore{io: io, outline: outline}
}

// SaveSummary Lưu tóm tắt chương vào tóm tắt/{ch}.json.
func (s *SummaryStore) SaveSummary(sum domain.ChapterSummary) error {
	return s.io.WriteJSON(fmt.Sprintf("summaries/%02d.json", sum.Chapter), sum)
}

// LoadSummary đọc tóm tắt của chương được chỉ định.
func (s *SummaryStore) LoadSummary(chapter int) (*domain.ChapterSummary, error) {
	var sum domain.ChapterSummary
	if err := s.io.ReadJSON(fmt.Sprintf("summaries/%02d.json", chapter), &sum); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &sum, nil
}

// LoadRecentSummaries tải bản tóm tắt của các chương gần đây nhất trước chương hiện tại.
func (s *SummaryStore) LoadRecentSummaries(current, count int) ([]domain.ChapterSummary, error) {
	var result []domain.ChapterSummary
	start := max(current-count, 1)
	for ch := start; ch < current; ch++ {
		sum, err := s.LoadSummary(ch)
		if err != nil {
			return nil, err
		}
		if sum != nil {
			result = append(result, *sum)
		}
	}
	return result, nil
}

// SaveArcSummary Lưu bản tóm tắt cấp độ cung.
func (s *SummaryStore) SaveArcSummary(sum domain.ArcSummary) error {
	return s.io.WriteJSON(fmt.Sprintf("summaries/arc-v%02da%02d.json", sum.Volume, sum.Arc), sum)
}

// HasArcSummary Kiểm tra xem cung đã chỉ định có bản tóm tắt được lưu hay không. Việc không đọc được xử lý là "chưa được lưu".
func (s *SummaryStore) HasArcSummary(volume, arc int) bool {
	sum, err := s.LoadArcSummary(volume, arc)
	return err == nil && sum != nil
}

// HasVolumeSummary Kiểm tra xem tập đĩa được chỉ định có bản tóm tắt được lưu hay không. Việc không đọc được xử lý là "chưa được lưu".
func (s *SummaryStore) HasVolumeSummary(volume int) bool {
	sum, err := s.LoadVolumeSummary(volume)
	return err == nil && sum != nil
}

// LoadArcSummary đọc tóm tắt của cung đã chỉ định.
func (s *SummaryStore) LoadArcSummary(volume, arc int) (*domain.ArcSummary, error) {
	var sum domain.ArcSummary
	if err := s.io.ReadJSON(fmt.Sprintf("summaries/arc-v%02da%02d.json", volume, arc), &sum); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &sum, nil
}

// LoadArcSummaries tải tất cả các bản tóm tắt cung hiện có trong một tập.
func (s *SummaryStore) LoadArcSummaries(volume int) ([]domain.ArcSummary, error) {
	maxArc := s.arcCountForVolume(volume)
	var result []domain.ArcSummary
	for arc := 1; arc <= maxArc; arc++ {
		sum, err := s.LoadArcSummary(volume, arc)
		if err != nil {
			return nil, err
		}
		if sum != nil {
			result = append(result, *sum)
		}
	}
	return result, nil
}

// SaveVolumeSummary Lưu tóm tắt mức âm lượng.
func (s *SummaryStore) SaveVolumeSummary(sum domain.VolumeSummary) error {
	return s.io.WriteJSON(fmt.Sprintf("summaries/vol-v%02d.json", sum.Volume), sum)
}

// LoadVolumeSummary Đọc bản tóm tắt của ổ đĩa được chỉ định.
func (s *SummaryStore) LoadVolumeSummary(volume int) (*domain.VolumeSummary, error) {
	var sum domain.VolumeSummary
	if err := s.io.ReadJSON(fmt.Sprintf("summaries/vol-v%02d.json", volume), &sum); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &sum, nil
}

// LoadAllVolumeSummaries tải tất cả các bản tóm tắt tập hiện có.
func (s *SummaryStore) LoadAllVolumeSummaries() ([]domain.VolumeSummary, error) {
	maxVol := s.volumeCount()
	var result []domain.VolumeSummary
	for vol := 1; vol <= maxVol; vol++ {
		sum, err := s.LoadVolumeSummary(vol)
		if err != nil {
			return nil, err
		}
		if sum != nil {
			result = append(result, *sum)
		}
	}
	return result, nil
}

// FindCharacterAppearances Batch tìm số chương xuất hiện cuối cùng của nhiều ký tự.
func (s *SummaryStore) FindCharacterAppearances(names []string, endChapter, recentWindow int) map[string]int {
	result := make(map[string]int, len(names))
	remaining := make(map[string]struct{}, len(names))
	for _, n := range names {
		remaining[n] = struct{}{}
	}
	for ch := endChapter - recentWindow; ch >= 1; ch-- {
		if len(remaining) == 0 {
			break
		}
		sum, err := s.LoadSummary(ch)
		if err != nil || sum == nil {
			continue
		}
		for _, c := range sum.Characters {
			if _, need := remaining[c]; need {
				result[c] = ch
				delete(remaining, c)
			}
		}
	}
	return result
}

func (s *SummaryStore) volumeCount() int {
	volumes, err := s.outline.LoadLayeredOutline()
	if err == nil && len(volumes) > 0 {
		return len(volumes)
	}
	return 20
}

func (s *SummaryStore) arcCountForVolume(volume int) int {
	volumes, err := s.outline.LoadLayeredOutline()
	if err == nil {
		for _, v := range volumes {
			if v.Index == volume {
				return len(v.Arcs)
			}
		}
	}
	return 20
}
