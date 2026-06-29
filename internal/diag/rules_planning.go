package diag

import (
	"fmt"
	"strings"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
)

// StaleForeshadow phát hiện những điềm báo dài chưa được nâng cao.
func StaleForeshadow(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Foreshadow) == 0 {
		return nil
	}
	latest := snap.LatestCompleted()
	threshold := staleForeshadowThreshold(snap.CompletedCount())

	var stale []string
	for _, f := range snap.Foreshadow {
		if f.Status != "planted" {
			continue
		}
		gap := latest - f.PlantedAt
		if gap > threshold {
			stale = append(stale, fmt.Sprintf("%s (ch%d đã bị chôn vùi, chương %d đã qua)", f.ID, f.PlantedAt, gap))
		}
	}
	if len(stale) == 0 {
		return nil
	}
	return []Finding{{
		Rule:       "StaleForeshadow",
		Category:   CatPlanning,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "context.foreshadow",
		Title:      fmt.Sprintf("Điềm báo bị đình trệ: Chương %d vượt quá chương %d nhưng không nâng cao", len(stale), threshold),
		Evidence:   strings.Join(stale, "; "),
		Suggestion: "Việc tải lời nhắc báo trước của tiểu thuyết_context có thể không hiệu quả hoặc lời nhắc của Nhà văn có thể thiếu hướng dẫn để nâng cao báo trước. Kiểm tra foreshadow_ledger và logic chèn ngữ cảnh.",
	}}
}

// La bàn phát hiện CompassDrift đã lâu không được cập nhật.
func CompassDrift(snap *Snapshot) []Finding {
	if snap.Progress == nil || !snap.Progress.Layered {
		return nil
	}
	if snap.Compass == nil {
		if snap.CompletedCount() > 5 {
			return []Finding{{
				Rule:       "CompassDrift",
				Category:   CatPlanning,
				Severity:   SevWarning,
				Confidence: ConfMedium,
				AutoLevel:  AutoNone,
				Target:     "prompt.architect",
				Title:      "Chế độ truyện dài thiếu la bàn",
				Evidence:   fmt.Sprintf("layered=true, completed=%d, compass=nil", snap.CompletedCount()),
				Suggestion: "Kiến trúc sư nên tạo la bàn trong quá trình lập kế hoạch ban đầu. Kiểm tra xem Architect-long.md có chứa hướng dẫn tạo la bàn hay không.",
			}}
		}
		return nil
	}

	gap := snap.LatestCompleted() - snap.Compass.LastUpdated
	if gap <= ThresholdCompassDrift {
		return nil
	}
	return []Finding{{
		Rule:       "CompassDrift",
		Category:   CatPlanning,
		Severity:   SevInfo,
		Confidence: ConfLow,
		AutoLevel:  AutoNone,
		Target:     "prompt.architect",
		Title:      fmt.Sprintf("Chương Compass %d chưa được cập nhật", gap),
		Evidence:   fmt.Sprintf("last_updated=ch%d, latest=ch%d, open_threads=%d", snap.Compass.LastUpdated, snap.LatestCompleted(), len(snap.Compass.OpenThreads)),
		Suggestion: "Kiến trúc sư nên cập nhật la bàn ở ranh giới cung/khối. Kiểm tra Architect-long.md để biết chỉ thị cập nhật la bàn.",
	}}
}

// OutlineExhaused phát hiện ra rằng dàn ý đã cạn kiệt nhưng tiểu thuyết vẫn chưa hoàn thành.
func OutlineExhausted(snap *Snapshot) []Finding {
	if snap.Progress == nil {
		return nil
	}
	p := snap.Progress
	if p.Phase == domain.PhaseComplete || p.Phase == domain.PhaseInit {
		return nil
	}

	completed := snap.CompletedCount()
	if completed == 0 {
		return nil
	}

	outlinedCount := p.TotalChapters
	if outlinedCount <= 0 {
		outlinedCount = len(snap.Outline)
	}
	if outlinedCount <= 0 {
		return nil
	}

	if completed < outlinedCount {
		return nil
	}

	return []Finding{{
		Rule:       "OutlineExhausted",
		Category:   CatPlanning,
		Severity:   SevCritical,
		Confidence: ConfHigh,
		AutoLevel:  AutoSafe,
		Target:     "runtime.recovery",
		Title:      fmt.Sprintf("Đề cương đã hết: Chương %d đã hoàn thành >= Chương %d đã được lên kế hoạch", completed, outlinedCount),
		Evidence:   fmt.Sprintf("phase=%s, completed=%d, outlined=%d", p.Phase, completed, outlinedCount),
		Suggestion: "Tín hiệu âm lượng mở rộng/mới có thể không phát ra. Kiểm tra chiến lược gửi phía máy chủ và logic khôi phục để xác nhận xem tính năng phát hiện ranh giới vòng cung, Expand_arc hoặcappend_volume có được thực thi bình thường hay không.",
	}}
}

// ThiếuTóm tắt Phát hiện các chương đã hoàn thành bị thiếu tóm tắt.
func MissingSummaries(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Progress.CompletedChapters) == 0 {
		return nil
	}

	var missing []int
	for _, ch := range snap.Progress.CompletedChapters {
		if _, ok := snap.Summaries[ch]; !ok {
			missing = append(missing, ch)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []Finding{{
		Rule:       "MissingSummaries",
		Category:   CatPlanning,
		Severity:   SevWarning,
		Confidence: ConfHigh,
		AutoLevel:  AutoNone,
		Target:     "runtime.flow",
		Title:      fmt.Sprintf("Thiếu tóm tắt: Chương %d không có tóm tắt", len(missing)),
		Evidence:   fmt.Sprintf("missing=[%s]", intsToStr(missing)),
		Suggestion: "Tóm tắt là chìa khóa cho tính liên tục theo ngữ cảnh. Kiểm tra xem logic viết tóm tắt của commit_chapter có hoạt động tốt không.",
	}}
}
