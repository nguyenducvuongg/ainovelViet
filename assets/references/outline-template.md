# Mẫu kế hoạch phác thảo

Mục đích của mẫu này không phải là nén tất cả các tác phẩm thành một độ dài cố định mà là để giúp xác định mức độ của tác phẩm trước rồi chọn độ chi tiết của phác thảo.

## Bước một: đầu tiên xác định độ dài của tác phẩm

### Truyện ngắn/Truyện một tập

- Áp dụng cho: xung đột đơn lẻ, mục tiêu đơn lẻ, ít ký tự, kết thúc tập trung
- Độ dài tham khảo: 8-25 chương
- Định dạng khuyên dùng: Flat `outline`

### Truyện trung bình/Nhiều giai đoạn

- Áp dụng: Có các nâng cấp theo giai đoạn, một số nhánh và mối quan hệ nhân vật sẽ thay đổi.
- Độ dài tham khảo: 25-60 chương
- Các định dạng được đề xuất: `outline` phẳng hoặc phân lớp nhẹ

### Truyện dài/Internet

- Áp dụng: Chủ đề đương nhiên có chỗ để nâng cấp liên tục, căng thẳng trong mối quan hệ lâu dài, mục tiêu nhiều giai đoạn, thế giới có thể mở rộng, bí ẩn dài hạn hoặc đường phát triển dài hạn
- Độ dài tham khảo: 80-200+ chương
- Định dạng khuyến nghị: `layered_outline` phân cấp

## Bước 2: Xác định xem có nên sử dụng phác thảo phân cấp hay không

Miễn là đáp ứng bất kỳ hai điều kiện nào sau đây, `layered_outline` sẽ được sử dụng trước:

- Thế giới quan cần được mở ra từng bước một thay vì giải thích tất cả cùng một lúc
- Sự phát triển của nhân vật chính không phải là một bước nhảy mà là nâng cấp nhiều giai đoạn
- Mối quan hệ nhân vật sẽ tiếp tục thay đổi theo nhiều giai đoạn
- Có nhiều loại xung đột chính ở giai đoạn giữa và cuối
- Yêu cầu chuyển đổi nhiều bản đồ/lực lượng/danh tính/mục tiêu
-Chủ đề rõ ràng giống một cuốn tiểu thuyết thương mại nhiều kỳ hơn là một câu chuyện một tập.

## Bước 3: Khi viết một cuốn tiểu thuyết dài, đừng chỉ làm “tóm tắt chương sách”

Trình tự được đề xuất cho việc lập kế hoạch dài hạn là:

1. Điểm bán hàng và sự khác biệt của công trình
2. Động cơ câu chuyện dài hạn
3. Chủ đề và nâng cấp ở cấp độ âm lượng
4. Mục tiêu ở cấp độ vòng cung và chuyển tiếp giai đoạn
5. Các sự kiện và móc nối cấp chương

Cách tiếp cận sai:

- Đầu tiên hãy viết dàn ý 20 chương, sau đó mạnh mẽ kéo dài nó ra
- Mỗi tập lặp lại "gặp địch - mạnh hơn - thay đổi bản đồ"
- Chỉ nâng cấp dòng chính, không cần nâng cấp
- Rút ra tất cả những bí mật lớn ở giai đoạn đầu và bạn chỉ có thể lặp lại quy trình ở giai đoạn giữa và cuối

## Mẫu phác thảo phẳng (ngắn/trung bình)

```json
[
  {
    "chapter": 1,
    "title": "Tiêu đề chương",
    "core_event": "Các sự kiện cốt lõi của chương này",
    "hook": "móc cuối chương",
    "scenes": ["bối cảnh1", "bối cảnh2", "bối cảnh3"]
  }
]
```

## Mẫu phác thảo nhiều lớp (bài viết dài - mở rộng cuộn hai lớp)

Quy hoạch ban đầu áp dụng cách cuộn hai lớp: 2 tập đầu tiên có hình vòng cung khung xương và các tập còn lại là khối khung xương; phần đầu tiên có các chương chi tiết.

```json
[
  {
    "index": 1,
    "title": "Tiêu đề tập 1",
    "theme": "Xung đột cốt lõi mới trong tập này/chủ đề",
    "arcs": [
      {
        "index": 1,
        "title": "Vòng cung đầu tiên (mở rộng)",
        "goal": "Mục tiêu địa phương, sự phản kháng và chuyển đổi",
        "chapters": [
          {"chapter": 1, "title": "Tiêu đề chương", "core_event": "sự kiện cốt lõi", "hook": "móc cuối chương", "scenes": ["bối cảnh1", "bối cảnh2"]}
        ]
      },
      {
        "index": 2,
        "title": "Cung thứ hai (cung xương)",
        "goal": "Tổng quan về các mục tiêu cho phần này",
        "estimated_chapters": 12,
        "chapters": []
      }
    ]
  },
  {
    "index": 2,
    "title": "Tiêu đề tập 2",
    "theme": "Chủ đề tập 2",
    "arcs": [
      {"index": 1, "title": "tiêu đề vòng cung", "goal": "mục tiêu vòng cung", "estimated_chapters": 15, "chapters": []},
      {"index": 2, "title": "tiêu đề vòng cung", "goal": "mục tiêu vòng cung", "estimated_chapters": 10, "chapters": []}
    ]
  },
  {
    "index": 3,
    "title": "Tiêu đề của Tập 3 (Tập Skeleton)",
    "theme": "Hướng chủ đề tập 3",
    "estimated_chapters": 60,
    "arcs": []
  }
]
```

- Mở rộng cấp độ cung: Khi viết tiến tới cung khung, Kiến trúc sư mở rộng các chương chi tiết của cung
- Mở rộng cấp độ tập: Khi viết tiến tới tập khung xương, Kiến trúc sư mở rộng cấu trúc vòng cung + chương đầu tiên của tập

## Danh sách kiểm tra cấp giấy dài

Mỗi tập phải trả lời:

- Thông tin thế giới mới nào được thêm vào trong tập này?
- Xung đột cốt lõi nào đã được nâng cấp trong tập này?
- Tập này khiến nhân vật chính được và mất gì?
- Tập này thay đổi mối quan hệ của nhân vật chính như thế nào?
- Hết tập này tại sao truyện lại phải chuyển sang tập tiếp theo?

## Danh sách kiểm tra cấp độ vòng cung dài

Mỗi cung phải được trả lời:

- Mục tiêu rõ ràng của phần này là gì?
- Sự phản kháng đến từ ai, quy luật nào và với cái giá như thế nào?
- Bước ngoặt là gì?
- Những trạng thái nào đã thay đổi không thể đảo ngược sau khi kết thúc phần này?

## Danh sách kiểm tra cấp độ chương

- Mỗi chương phải phục vụ mục tiêu của arc
- Mỗi chương phải chứa một sự kiện không thể xóa được
-Móc câu phải đa dạng và không chỉ dựa vào việc “khám phá bí mật”.
- Những chương đầu không thể chỉ “giới thiệu thế giới”, phải đồng thời đề cao nhân vật và xung đột
