package diag

import (
	"fmt"
	"strings"
)

// GhostCharacter phát hiện nhân vật cốt lõi/quan trọng đã lâu không xuất hiện.
func GhostCharacter(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Characters) == 0 || len(snap.Summaries) == 0 {
		return nil
	}
	completed := snap.CompletedCount()
	if completed < 5 {
		return nil
	}

	// Tính số chương mà mỗi ký tự xuất hiện lần cuối
	lastSeen := make(map[string]int)
	for ch, s := range snap.Summaries {
		for _, name := range s.Characters {
			if ch > lastSeen[name] {
				lastSeen[name] = ch
			}
		}
	}

	threshold := completed / 3
	if threshold < 5 {
		threshold = 5
	}
	latest := snap.LatestCompleted()

	var ghosts []string
	for _, c := range snap.Characters {
		if c.Tier != "core" && c.Tier != "important" {
			continue
		}
		seen, ok := lastSeen[c.Name]
		if !ok {
			// Đồng thời kiểm tra bí danh
			for _, alias := range c.Aliases {
				if s, exists := lastSeen[alias]; exists && s > seen {
					seen = s
					ok = true
				}
			}
		}
		gap := latest - seen
		if !ok {
			ghosts = append(ghosts, fmt.Sprintf("%s (không bao giờ xuất hiện trong bản tóm tắt)", c.Name))
		} else if gap > threshold {
			ghosts = append(ghosts, fmt.Sprintf("%s (ch%d xuất hiện cuối cùng và vắng mặt trong chương %d)", c.Name, seen, gap))
		}
	}
	if len(ghosts) == 0 {
		return nil
	}
	return []Finding{{
		Rule:       "GhostCharacter",
		Category:   CatContext,
		Severity:   SevInfo,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "context.characters",
		Title:      fmt.Sprintf("Nhân vật biến mất: Các nhân vật cốt lõi %d vắng mặt trong thời gian dài", len(ghosts)),
		Evidence:   strings.Join(ghosts, "; "),
		Suggestion: "Có lẽ người viết đã quên mất nhân vật này. Hãy cân nhắc gửi hướng dẫn can thiệp trực tiếp vào hộp nhập để giới thiệu lại ký tự hoặc hạ cấp cấp của ký tự đó trong character.json.",
	}}
}

// TimelineGaps phát hiện các sự kiện dòng thời gian bị thiếu cho các chương đã hoàn thành.
func TimelineGaps(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Progress.CompletedChapters) == 0 {
		return nil
	}
	if len(snap.Timeline) == 0 && snap.CompletedCount() > 0 {
		return []Finding{{
			Rule:       "TimelineGaps",
			Category:   CatContext,
			Severity:   SevInfo,
			Confidence: ConfMedium,
			AutoLevel:  AutoNone,
			Target:     "context.timeline",
			Title:      "Dòng thời gian trống",
			Evidence:   fmt.Sprintf("completed=%d, timeline_events=0", snap.CompletedCount()),
			Suggestion: "Trích xuất dòng thời gian cho commit_chapter có thể không có hiệu lực. Kiểm tra xem đầu ra Writer có chứa trường dòng thời gian hay không.",
		}}
	}

	// Tạo Chương→Bản đồ sự kiện
	chaptersWithEvents := make(map[int]bool)
	for _, e := range snap.Timeline {
		chaptersWithEvents[e.Chapter] = true
	}

	var missing []int
	for _, ch := range snap.Progress.CompletedChapters {
		if !chaptersWithEvents[ch] {
			missing = append(missing, ch)
		}
	}
	// Cho phép thiếu một số vật phẩm (một số chương chuyển tiếp có thể thực sự không có sự kiện lớn)
	if len(missing) == 0 || float64(len(missing))/float64(snap.CompletedCount()) < ThresholdTimelineGapRate {
		return nil
	}
	return []Finding{{
		Rule:       "TimelineGaps",
		Category:   CatContext,
		Severity:   SevInfo,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "context.timeline",
		Title:      fmt.Sprintf("Khoảng cách dòng thời gian: Chương %d không có bản ghi sự kiện", len(missing)),
		Evidence:   fmt.Sprintf("missing=[%s]", intsToStr(missing)),
		Suggestion: "Trích xuất dòng thời gian cho commit_chapter có thể không thành công một phần. Kiểm tra định dạng trường dòng thời gian của đầu ra Writer.",
	}}
}

// Mối quan hệStagnation Phát hiện dữ liệu mối quan hệ đã ngừng cập nhật.
func RelationshipStagnation(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Relationships) == 0 {
		return nil
	}
	completed := snap.CompletedCount()
	if completed < 6 {
		return nil
	}

	// Tìm chương mới nhất về dữ liệu quan hệ
	latestRelCh := 0
	for _, r := range snap.Relationships {
		if r.Chapter > latestRelCh {
			latestRelCh = r.Chapter
		}
	}

	// Nếu dữ liệu quan hệ mới nhất nằm trong top 1/3 thì được xác định là trì trệ.
	cutoff := snap.LatestCompleted() - completed/3
	if latestRelCh >= cutoff {
		return nil
	}
	return []Finding{{
		Rule:       "RelationshipStagnation",
		Category:   CatContext,
		Severity:   SevInfo,
		Confidence: ConfLow,
		AutoLevel:  AutoNone,
		Target:     "context.relationships",
		Title:      fmt.Sprintf("Sự đình trệ dữ liệu quan hệ: cập nhật mới nhất trong Chương %d", latestRelCh),
		Evidence:   fmt.Sprintf("relationship_entries=%d, latest_update=ch%d, latest_completed=ch%d", len(snap.Relationships), latestRelCh, snap.LatestCompleted()),
		Suggestion: "Bản cập nhật mối quan hệ cho commit_chapter có thể đã ngừng hoạt động hoặc các mối quan hệ trong câu chuyện thực sự không thay đổi. Kiểm tra trường mối quan hệ của đầu ra Writer.",
	}}
}
