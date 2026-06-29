package diag

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ChronicLowDimension phát hiện điểm thấp liên tục trong một khía cạnh đánh giá trên nhiều chương.
func ChronicLowDimension(snap *Snapshot) []Finding {
	if len(snap.Reviews) < 2 {
		return nil
	}

	dimSums := make(map[string]float64)
	dimCounts := make(map[string]int)
	for _, r := range snap.Reviews {
		for _, d := range r.Dimensions {
			dimSums[d.Dimension] += float64(d.Score)
			dimCounts[d.Dimension]++
		}
	}

	var findings []Finding
	for name, sum := range dimSums {
		count := dimCounts[name]
		if count < 2 {
			continue
		}
		avg := sum / float64(count)
		if avg >= ThresholdDimScoreLow {
			continue
		}
		findings = append(findings, Finding{
			Rule:       "ChronicLowDimension",
			Category:   CatQuality,
			Severity:   SevWarning,
			Confidence: ConfMedium,
			AutoLevel:  AutoNone,
			Target:     "prompt.writer",
			Title:      fmt.Sprintf("Thứ nguyên [%s] Điểm thấp liên tục (%.0f trung bình)", name, avg),
			Evidence:   fmt.Sprintf("Tổng số %d đánh giá, với điểm trung bình là %.1f", count, avg),
			Suggestion: fmt.Sprintf("Kiểm tra xem hướng dẫn dành cho %s trong lời nhắc của Người viết có rõ ràng hay không hoặc tiêu chí chấm điểm cho %s trong lời nhắc của Người biên tập có hợp lý hay không.", name, name),
		})
	}
	return findings
}

// ContractMissPattern Phát hiện tỷ lệ thực hiện hợp đồng quá thấp.
func ContractMissPattern(snap *Snapshot) []Finding {
	if len(snap.Reviews) == 0 {
		return nil
	}

	var total, missed int
	var missedChapters []string
	for ch, r := range snap.Reviews {
		total++
		if r.ContractStatus == "partial" || r.ContractStatus == "missed" {
			missed++
			missedChapters = append(missedChapters, fmt.Sprintf("ch%d", ch))
		}
	}
	if total == 0 {
		return nil
	}
	rate := float64(missed) / float64(total)
	if rate <= ThresholdContractMissRate {
		return nil
	}
	return []Finding{{
		Rule:       "ContractMissPattern",
		Category:   CatQuality,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("Tỷ lệ thực hiện hợp đồng thấp (không đạt được %.0f%%)", rate*100),
		Evidence:   fmt.Sprintf("Chưa đạt: [%s], tổng %d/%d", strings.Join(missedChapters, ", "), missed, total),
		Suggestion: "Người viết có thể chưa đọc hợp đồng hoặc hợp đồng require_beats có thể quá mạnh mẽ. Kiểm tra sự phối hợp của plan_chapter và writer.md.",
	}}
}

// HookWeakChain phát hiện rằng điểm hook của chương liên tục yếu.
func HookWeakChain(snap *Snapshot) []Finding {
	if len(snap.Reviews) < ThresholdHookWeakChain {
		return nil
	}

	chapters := sortedChapterReviews(snap)
	var weakChain []int
	for _, ch := range chapters {
		review := snap.Reviews[ch]
		if review == nil || review.Scope != "chapter" {
			continue
		}
		hook := review.Dimension("hook")
		if hook == nil || hook.Score >= ThresholdHookWeakScore {
			if len(weakChain) >= ThresholdHookWeakChain {
				break
			}
			weakChain = weakChain[:0]
			continue
		}
		weakChain = append(weakChain, ch)
	}
	if len(weakChain) < ThresholdHookWeakChain {
		return nil
	}

	var parts []string
	for _, ch := range weakChain {
		if hook := snap.Reviews[ch].Dimension("hook"); hook != nil {
			parts = append(parts, fmt.Sprintf("ch%d(%d)", ch, hook.Score))
		}
	}
	return []Finding{{
		Rule:       "HookWeakChain",
		Category:   CatQuality,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("Móc cuối chương liên tục yếu (các chương %d liên tiếp)", len(weakChain)),
		Evidence:   strings.Join(parts, ", "),
		Suggestion: "Kiểm tra xem việc thực thi hook_goal trong writer.md có rõ ràng hay không. Nếu cần, hãy làm rõ mong muốn đọc chương này trong plan_chapter và hiệu chỉnh tiêu chuẩn chứng minh của Biên tập viên về các điểm móc nối.",
	}}
}

// PayoffMissPattern phát hiện các chương có điểm hoàn trả đã lâu không được vinh danh.
func PayoffMissPattern(snap *Snapshot) []Finding {
	var total, missed int
	var details []string
	for ch, plan := range snap.Plans {
		if plan == nil || len(plan.Contract.PayoffPoints) == 0 {
			continue
		}
		review := snap.Reviews[ch]
		if review == nil {
			continue
		}
		total++
		if review.ContractStatus == "partial" || review.ContractStatus == "missed" {
			missed++
			details = append(details, fmt.Sprintf("ch%d(Phần thưởng vật phẩm %d)", ch, len(plan.Contract.PayoffPoints)))
		}
	}
	if total < 2 {
		return nil
	}
	rate := float64(missed) / float64(total)
	if rate <= ThresholdPayoffMissRate {
		return nil
	}
	sort.Strings(details)
	return []Finding{{
		Rule:       "PayoffMissPattern",
		Category:   CatQuality,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("Tỷ lệ đổi điểm thú vị/điểm cốt truyện thấp (không đạt %.0f%%)", rate*100),
		Evidence:   fmt.Sprintf("Các chương chưa hoàn thành: tổng cộng [%s], %d/%d", strings.Join(details, ", "), missed, total),
		Suggestion: "Kiểm tra xem điểm thưởng của chương_kế hoạch có quá nhiều hay quá trống không và đảm bảo rằng Người viết tôn trọng chúng một cách rõ ràng trong văn bản thay vì chỉ báo trước chúng.",
	}}
}

// ExtremeRewrites phát hiện tỷ lệ ghi lại quá mức.
func ExcessiveRewrites(snap *Snapshot) []Finding {
	if len(snap.Reviews) < 2 {
		return nil
	}

	var total, rewrites int
	for _, r := range snap.Reviews {
		total++
		if r.Verdict == "rewrite" {
			rewrites++
		}
	}
	if total == 0 {
		return nil
	}
	rate := float64(rewrites) / float64(total)
	if rate <= ThresholdRewriteRate {
		return nil
	}
	return []Finding{{
		Rule:       "ExcessiveRewrites",
		Category:   CatQuality,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.editor",
		Title:      fmt.Sprintf("Tốc độ ghi đè quá cao (%d/%d = %.0f%%)", rewrites, total, rate*100),
		Evidence:   fmt.Sprintf("Tổng số lượt đánh giá %d và lượt viết lại %d", total, rewrites),
		Suggestion: "Người viết tiếp tục sản xuất nội dung dưới ngưỡng của Biên tập viên. Kiểm tra xem tiêu chuẩn chất lượng của lời nhắc Người viết có phù hợp với tiêu chuẩn đánh giá của Người biên tập hay không.",
	}}
}

// WordCountAnomaly phát hiện sự bất thường về số từ trong chương.
func WordCountAnomaly(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Progress.ChapterWordCounts) < 3 {
		return nil
	}
	wc := snap.Progress.ChapterWordCounts

	var sum float64
	for _, w := range wc {
		sum += float64(w)
	}
	avg := sum / float64(len(wc))
	if avg == 0 {
		return nil
	}

	var anomalies []string
	for ch, w := range wc {
		ratio := float64(w) / avg
		if ratio < ThresholdWordShortRatio {
			anomalies = append(anomalies, fmt.Sprintf("ch%d(từ %d,%.0f%%)", ch, w, ratio*100))
		} else if ratio > ThresholdWordLongRatio {
			anomalies = append(anomalies, fmt.Sprintf("ch%d(từ %d,%.0f%%)", ch, w, ratio*100))
		}
	}
	if len(anomalies) == 0 {
		return nil
	}
	return []Finding{{
		Rule:       "WordCountAnomaly",
		Category:   CatQuality,
		Severity:   SevInfo,
		Confidence: ConfLow,
		AutoLevel:  AutoNone,
		Target:     "context.window",
		Title:      fmt.Sprintf("Số từ bất thường trong các chương (trung bình %d từ)", int(math.Round(avg))),
		Evidence:   strings.Join(anomalies, "; "),
		Suggestion: "Các chương rất ngắn có thể bị cắt bớt đầu ra (giới hạn mã thông báo) và các chương rất dài có thể tiêu tốn quá nhiều cửa sổ ngữ cảnh. Kiểm tra cấu hình max_tokens của mô hình.",
	}}
}

func sortedChapterReviews(snap *Snapshot) []int {
	chapters := make([]int, 0, len(snap.Reviews))
	for ch := range snap.Reviews {
		chapters = append(chapters, ch)
	}
	sort.Ints(chapters)
	return chapters
}
