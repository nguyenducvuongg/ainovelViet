package diag

import (
	"fmt"
	"sort"

	"github.com/nguyenducvuongg/ainovelViet/internal/store"
)

// ── Ngưỡng chẩn đoán ───────────────────── ─────────────────────

const (
	ThresholdDimScoreLow      = 70  // ChronicLowDimension: Cảnh báo khi kích thước trung bình thấp hơn giá trị này
	ThresholdContractMissRate = 0.3 // ContractMissPattern: Giới hạn trên của tỷ lệ thất bại hợp đồng
	ThresholdRewriteRate      = 0.5 // Viết lại quá mức: giới hạn trên của tốc độ ghi lại
	ThresholdWordShortRatio   = 0.4 // WordCountAnomaly: Số từ dưới mức trung bình được coi là bất thường.
	ThresholdWordLongRatio    = 2.5 // WordCountAnomaly: Số từ trên mức trung bình được coi là bất thường.
	ThresholdHookWeakScore    = 75  // HookWeakChain: hook dưới điểm này được coi là yếu
	ThresholdHookWeakChain    = 3   // HookWeakChain: Ngưỡng chương yếu liên tục
	ThresholdPayoffMissRate   = 0.4 // PayoffMissPattern: giới hạn trên của tỷ lệ thanh toán chưa được thanh toán
	ThresholdCompassDrift     = 15  // CompassDrift: Compass chưa cập nhật giới hạn trên của chương
	ThresholdTimelineGapRate  = 0.3 // Khoảng trống thời gian: Thiếu giới hạn trên về dung sai tỷ lệ
	ThresholdForeshadowMin    = 8   // StaleForeshadow: Số chương tối thiểu để báo trước sự trì trệ
)

// tất cả các quy tắc được sắp xếp theo quy trình → chất lượng → lập kế hoạch → bối cảnh.
var allRules = []RuleFunc{
	// Flow
	InvalidPendingRewrites,
	RewritePendingPressure,
	OrphanedSteer,
	PhaseFlowMismatch,
	ChapterGaps,
	// Quality
	ChronicLowDimension,
	ContractMissPattern,
	HookWeakChain,
	PayoffMissPattern,
	ExcessiveRewrites,
	WordCountAnomaly,
	// Planning
	StaleForeshadow,
	CompassDrift,
	OutlineExhausted,
	MissingSummaries,
	// Context
	GhostCharacter,
	TimelineGaps,
	RelationshipStagnation,
}

// Phân tích là điểm vào duy nhất của hệ thống chẩn đoán.
func Analyze(s *store.Store) Report {
	snap := Load(s)

	var findings []Finding
	for _, e := range snap.LoadErrors {
		findings = append(findings, Finding{
			Rule:       "LoadError",
			Category:   CatFlow,
			Severity:   SevWarning,
			Confidence: ConfHigh,
			AutoLevel:  AutoNone,
			Target:     "runtime.flow",
			Title:      fmt.Sprintf("Tải hiện vật không thành công: %s", e),
			Suggestion: "Tệp có thể bị hỏng hoặc không có đủ quyền và kết quả của các quy tắc chẩn đoán liên quan có thể không đầy đủ.",
		})
	}
	for _, rule := range allRules {
		findings = append(findings, rule(&snap)...)
	}
	sortFindings(findings)

	return Report{
		Stats:    buildStats(&snap),
		Findings: findings,
		Actions:  PlanActions(findings),
	}
}

func buildStats(snap *Snapshot) Stats {
	st := Stats{}
	if snap.Progress == nil {
		return st
	}
	p := snap.Progress
	st.CompletedChapters = len(p.CompletedChapters)
	st.TotalChapters = p.TotalChapters
	st.TotalWords = p.TotalWordCount
	st.Phase = string(p.Phase)
	st.Flow = string(p.Flow)

	if st.CompletedChapters > 0 {
		st.AvgWordsPerCh = st.TotalWords / st.CompletedChapters
	}

	if snap.RunMeta != nil {
		st.PlanningTier = string(snap.RunMeta.PlanningTier)
	}

	// Xem lại số liệu thống kê
	st.ReviewCount = len(snap.Reviews)
	var totalScore float64
	var dimCount int
	for _, r := range snap.Reviews {
		if r.Verdict == "rewrite" {
			st.RewriteCount++
		}
		for _, d := range r.Dimensions {
			totalScore += float64(d.Score)
			dimCount++
		}
	}
	if dimCount > 0 {
		st.AvgReviewScore = totalScore / float64(dimCount)
	}

	// Thống kê báo trước
	latest := snap.LatestCompleted()
	for _, f := range snap.Foreshadow {
		if f.Status == "planted" || f.Status == "advanced" {
			st.ForeshadowOpen++
			if f.Status == "planted" && latest-f.PlantedAt > staleForeshadowThreshold(st.CompletedChapters) {
				st.ForeshadowStale++
			}
		}
	}
	return st
}

// SortFindings Sắp xếp theo mức độ nghiêm trọng: quan trọng > cảnh báo > thông tin.
func sortFindings(findings []Finding) {
	order := map[Severity]int{SevCritical: 0, SevWarning: 1, SevInfo: 2}
	sort.SliceStable(findings, func(i, j int) bool {
		return order[findings[i].Severity] < order[findings[j].Severity]
	})
}

// staleForeshadowThreshold Tính ngưỡng độ cứng của staleshadow dựa trên tổng số chương.
func staleForeshadowThreshold(completedChapters int) int {
	t := completedChapters / 3
	if t < ThresholdForeshadowMin {
		return ThresholdForeshadowMin
	}
	return t
}
