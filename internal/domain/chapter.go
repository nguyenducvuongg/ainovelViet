package domain

import (
	"fmt"
	"unicode/utf8"
)

// ReviewInterval Khoảng thời gian xem xét toàn cầu (kích hoạt mỗi N chương).
const ReviewInterval = 5

// ShouldReview xác định xem có cần đánh giá toàn cầu hay không dựa trên số chương đã hoàn thành (chế độ ngắn/trung bình).
func ShouldReview(completedCount int) (bool, string) {
	if completedCount > 0 && completedCount%ReviewInterval == 0 {
		return true, fmt.Sprintf("Chương %d đã được hoàn thành, kích hoạt đánh giá toàn cầu", completedCount)
	}
	return false, ""
}

// ShouldArcReview xác định xem có cần xem xét cấp độ vòng cung/cấp âm lượng ở chế độ dạng dài hay không.
func ShouldArcReview(isArcEnd, isVolumeEnd bool, volume, arc int) (bool, string) {
	if isVolumeEnd {
		return true, fmt.Sprintf("Cung của Tập %d và Tập %d kết thúc (cuối tập), kích hoạt mức cung + xem xét mức âm lượng", volume, arc)
	}
	if isArcEnd {
		return true, fmt.Sprintf("Tập %d Chương %d kết thúc, quá trình xem xét cấp độ vòng cung được kích hoạt", volume, arc)
	}
	return false, ""
}

// WordCount đếm từ bằng rune.
func WordCount(content string) int {
	return utf8.RuneCountInString(content)
}
