package rules

import (
	"regexp"
	"strings"
)

// Kiểm tra dòng cuối cùng của sản phẩm được tích hợp sẵn của Lint: quét phần còn lại của cơ chế trong văn bản, bất kể quy tắc của người dùng và luôn được thực thi khi cam kết.
// Hợp đồng tương tự như Kiểm tra - chỉ trả về thông tin thực tế (quy tắc sắt thứ nhất), không chặn quy trình và do người đánh giá/người dùng quyết định.
//
// Ba loại hiện tại (tất cả đều bắt nguồn từ những sai sót thực nghiệm trong các sản phẩm chạy đường dài thực tế):
//   - markdown_residue: phần dư văn bản ** in đậm, # dòng tiêu đề nằm ngoài dòng đầu tiên (xuất txt sẽ hiển thị ký hiệu)
//   - non_cjk_fragments: các đoạn chữ cái Latinh liên tục (ngôn ngữ mô hình được trộn lẫn, chẳng hạn như văn bản tiếng Trung được trộn với "mẫu")
func Lint(text string) []Violation {
	var vs []Violation
	vs = appendMarkdownResidue(vs, text)
	vs = appendNonCJKFragments(vs, text)
	return vs
}

func appendMarkdownResidue(vs []Violation, text string) []Violation {
	if n := strings.Count(text, "**"); n > 0 {
		vs = append(vs, Violation{
			Rule:     "markdown_residue",
			Target:   "**",
			Actual:   n,
			Severity: SeverityWarning,
		})
	}
	headings := 0
	seenContent := false
	for line := range strings.SplitSeq(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// Tiêu đề # dòng không trống đầu tiên là định dạng hợp pháp của tệp chương (không được mã hóa cứng theo số dòng, chấp nhận các dòng trống ở đầu)
		first := !seenContent
		seenContent = true
		if !first && strings.HasPrefix(t, "#") {
			headings++
		}
	}
	if headings > 0 {
		vs = append(vs, Violation{
			Rule:     "markdown_residue",
			Target:   "#",
			Actual:   headings,
			Severity: SeverityWarning,
		})
	}
	return vs
}

var latinFragmentRe = regexp.MustCompile(`[A-Za-z]{2,}`)

// appendNonCJKFragments báo cáo tổng số đoạn chữ cái Latinh kèm theo ví dụ chống trùng lặp.
// Tiếng Anh pháp lý (tên thương hiệu/viết tắt) của các chủ đề hiện đại cũng sẽ đạt mức độ cảnh báo, được ban giám khảo xác định theo chủ đề.
func appendNonCJKFragments(vs []Violation, text string) []Violation {
	matches := latinFragmentRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return vs
	}
	seen := make(map[string]struct{})
	var examples []string
	for _, m := range matches {
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		if len(examples) < 3 {
			examples = append(examples, m)
		}
	}
	return append(vs, Violation{
		Rule:     "non_cjk_fragments",
		Target:   strings.Join(examples, "、"),
		Actual:   len(matches),
		Severity: SeverityWarning,
	})
}
