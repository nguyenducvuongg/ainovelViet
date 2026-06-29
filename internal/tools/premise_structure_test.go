package tools

import (
	"testing"

	"github.com/nguyenducvuongg/ainovelViet/internal/domain"
)

func TestParsePremiseSections(t *testing.T) {
	premise := `#tiền đề

## Chủ đề và giai điệu
Ảo tưởng phương Đông, tăng trưởng lạnh lùng và khó khăn.

## Định vị chủ đề
Dòng truyện giả tưởng phương Đông được nâng cấp nhắm đến những độc giả đang tìm kiếm sự phấn khích và thăng tiến trong mối quan hệ.

## Xung đột cốt lõi
Nhân vật chính phải lựa chọn giữa quy định của giáo phái và lương tâm cá nhân.

## Lượt chuyển tiếp giữa kỳ
Con đường tu luyện cũ đã trở nên kém hiệu quả và phải chuyển sang hệ thống cấm thuật.
`

	sections := parsePremiseSections(premise)
	if sections["chủ đề và giọng điệu"] == "" {
		t.Fatalf("phần chủ đề và giai điệu dự kiến, có %+v", sections)
	}
	if sections["Định vị chủ đề"] == "" {
		t.Fatalf("phần định vị chủ đề dự kiến, có %+v", sections)
	}
	if sections["xung đột cốt lõi"] == "" {
		t.Fatalf("phần xung đột cốt lõi dự kiến, có %+v", sections)
	}
	if sections["giữa lượt"] == "" {
		t.Fatalf("bí danh giữa lượt dự kiến ​​được chuẩn hóa thành giữa lượt, có %+v", sections)
	}
}

func TestPremiseStructure(t *testing.T) {
	premise := `## Chủ đề và giai điệu
Dòng chảy nâng cấp, lạnh lùng và cứng rắn hơn.

## Định vị chủ đề
Luồng nâng cấp

## Xung đột cốt lõi
xung đột

## Mục tiêu của nhân vật chính
mục tiêu

## Hướng đi cuối cùng
Đêm chung kết

## Vùng hạn chế viết
khu vực hạn chế

##Điểm bán hàng khác biệt
điểm bán hàng

## Móc phân biệt
cái móc

## Thực hiện cốt lõi những lời hứa
rút tiền

## Công cụ câu chuyện
động cơ

## Bước ngoặt đoạn giữa
bước ngoặt
`

	structure := premiseStructure(premise, domain.PlanningTierMid)
	if ready, _ := structure["template_ready"].(bool); !ready {
		t.Fatalf("expected template_ready, got %+v", structure)
	}
	missing, _ := structure["missing"].([]string)
	if len(missing) != 0 {
		t.Fatalf("expected no missing headings, got %+v", missing)
	}
}

func TestPremiseStructureShortAcceptsLegacyHeadingAlias(t *testing.T) {
	premise := `## Chủ đề và giai điệu
Cứu hộ áp lực cao cuộn đơn.

## Định vị chủ đề
Một cuộc phiêu lưu mật độ cao ngắn.

## Xung đột cốt lõi
Nhân vật chính phải giải cứu con tin trong một đêm.

## Mục tiêu của nhân vật chính
Giải cứu con tin và sống sót thoát ra ngoài.

## Hướng kết thúc
Hoàn thành nhiệm vụ nhưng phải trả giá.

## Vùng hạn chế viết
Việc tuần tự hóa thời kỳ tăng trưởng sẽ không được kéo dài.

##Điểm bán hàng khác biệt
Áp lực thời gian và sự đảo chiều liên tục.

## Móc phân biệt
Mỗi lựa chọn sẽ rút ngắn thời gian giải cứu.

## Thực hiện cốt lõi những lời hứa
Sự khẩn cấp, sự lựa chọn và sự đảo ngược.

##Tại sao cuốn sách này lại phù hợp với truyện ngắn/tập đơn?
Cả xung đột cốt lõi và cốt truyện của nhân vật đều có thể được hoàn thành trong một nhiệm vụ duy nhất.
`

	structure := premiseStructure(premise, domain.PlanningTierShort)
	if ready, _ := structure["template_ready"].(bool); !ready {
		t.Fatalf("expected short template_ready, got %+v", structure)
	}
}
