# Hướng dẫn quan sát

Khi chạy một cuốn tiểu thuyết, làm sao bạn biết liệu các cơ chế khác nhau có thực sự hoạt động hay không?

Tài liệu này không phải là bản sao của các quy tắc chẩn đoán mà hướng đến **hoạt động thực tế**: bạn đã đến Chương N, tệp nào sẽ được mở, trường nào cần xem và liệu nó khỏe mạnh hay bất thường.

---

## 1. Quy trình khắc phục sự cố chung

```
1. /diag                       # Chẩn đoán tự động, xem Findings huyện
2. cd output/{novel}/meta/     # trực tiếp cat hiện vật quan trọng
3. cat meta/sessions/coordinator.jsonl | tail  # Hãy nhìn vào những vòng cuối cùng LLM Hành vi
```

Các sự kiện không có trong `/diag` (bao gồm các mục "được chẩn đoán" được liệt kê trong tài liệu này) cần được kiểm tra thủ công ở bước 2-3.

### Báo cáo sự cố: Xuất chẩn đoán giải mẫn cảm

Mỗi lần `/diag` sẽ viết thêm một `output/{novel}/meta/diag-export.md` - một chẩn đoán **giải mẫn cảm** (văn bản/lời nhắc/suy nghĩ mới đã bị xóa, chỉ giữ lại khung hành vi: tên công cụ, chuỗi lỗi, số lần lặp lại, pha/luồng, bước bị mắc kẹt, phân loại lỗi nhật ký). Nếu bạn gặp phải sự cố vòng lặp/gián đoạn vô hạn, chỉ cần đăng tệp này lên sự cố GitHub và người bảo trì sẽ xác định vị trí của nó cho phù hợp mà không yêu cầu dữ liệu `output/` của người dùng.

---

## 2. Bảng cheat tạo tác chính

Sắp xếp theo "Các đường dẫn khắc phục sự cố phổ biến nhất khi xảy ra sự cố":

| Hiện vật | Đường dẫn | Xem gì | Khỏe mạnh | Không lành mạnh |
|---|---|---|---|---|
| Tiến độ | `meta/progress.json` | `phase` / `flow` / `completed_chapters` | pha tiến lên đơn điệu, dòng chảy nằm trong tập hợp pháp | pha đi ngược lại/dòng chảy bị kẹt ở một trạng thái nhất định |
| La bàn | `meta/compass.json` | Khoảng cách `last_updated` với chương mới nhất | Gap < 15 chương | gap > 15 chương (CompassDrift hit) |
| Danh sách vai phụ | `meta/cast_ledger.json` | Số lượng mục nhập / Tỷ lệ lấp đầy Brief_role / Tính nhất quán của tên | Xem §4 | Xem §4 |
| Sổ cái báo trước | `meta/foreshadow.json` | Số chương trì trệ dài nhất trong `status="planted"` | Số chương trong < Số chương/3 | >/3 (StaleForeshadow hit) |
| Đề cương | `meta/layered_outline.json` | Số chương chưa viết còn lại trong tập hiện tại | Mở rộng trước 1-2 chương | Chương hiện tại đã viết xong nhưng chương tiếp theo chưa có dàn ý (OutlineExhaused) |
| Lưu trữ nhân vật | `meta/characters.json` | Liệu nhân vật cốt lõi/quan trọng có thể được tìm thấy trong N tóm tắt chương cuối hay không | Có thể tìm thấy | Vắng mặt (Bản hit GhostCharacter) |
| Điểm kiểm tra | `meta/checkpoints.jsonl` | Hàng mới nhất của `step` có tương ứng với tiến trình hay không | Nhất quán | Không nhất quán (khôi phục sự cố không tự phục hồi) |
| Phiên điều phối viên | `meta/sessions/coordinator.jsonl` | 5-10 vòng cuối cùng của chế độ tool_call | Một vòng thăng tiến nhanh chóng | Cùng một công cụ điều chỉnh nhiều lần (vòng lặp bị kẹt) |

---

## 3. Quan sát bằng la bàn

**Thời gian sửa lỗi**: 2026-05-08 (cam kết `fix: update_compass Tự động điền công cụ last_updated`)

### Xem gì

```bash
cat output/{novel}/meta/compass.json
```

Ngữ nghĩa trường:
- `ending_direction`: hướng cuối cùng (phải nhất quán với đoạn "hướng cuối cùng" `premise.md`)
- `open_threads`: đường dài hoạt động (ranh giới mỗi tập được kiến ​​trúc sư thêm và xóa)
- `estimated_scale`: Tỷ lệ ước tính (chẳng hạn như “4-6 tập”, ranh giới của mỗi tập được cập nhật)
- `last_updated`: **Công cụ tự động điền** số chương hoàn thành tối đa tại thời điểm cập nhật (không còn dựa vào LLM để điền tự động)

### Phán quyết về sức khỏe

| Tín hiệu | Phán quyết |
|---|---|
| `last_updated` trong phạm vi `[latest-15, latest]` | sức khỏe |
| `last_updated` tụt hậu mới nhất hơn 15 chương | kiến trúc sư không cập nhật ở ranh giới cung/khối lượng - kiểm tra lời nhắc của kiến ​​trúc sư-long.md |
| `last_updated == 0` | **Dữ liệu bẩn trước khi sửa chữa**, bản cập nhật_compass tiếp theo sẽ tự phục hồi |
| `ending_direction` không khớp với phần "hướng cuối cùng" của tiền đề.md | kiến trúc sư đã bí mật thay đổi ý định của người dùng - ghi lại nó và quyết định xem có nên đóng băng trường này hay không (đối với các vấn đề về thiết kế, hãy xem todo.md) |

### Làm thế nào để xác minh rằng việc sửa chữa có hiệu quả?

So sánh trước và sau khi chạy một cuốn tiểu thuyết dài:
- **Trước khi sửa**: Sau khi chạy hơn 30 chương, `compass.last_updated` rất có thể là `0` hoặc số chương đầu.
- **Sau khi sửa chữa**: Mỗi lần kiến ​​trúc sư điều chỉnh `update_compass`, `last_updated` sẽ bị lớp công cụ ghi đè là mới nhất hiện tại

---

## 4. Quan sát danh sách diễn viên phụ (cast_ledger)

**Triển khai chức năng**: 2026-05-08 (cam kết `feat: Đã thêm danh sách diễn viên hỗ trợ để tự động theo dõi các nhân vật phụ`)

### Xem gì

```bash
cat output/{novel}/meta/cast_ledger.json | jq 'length'                     # Tổng số mục
cat output/{novel}/meta/cast_ledger.json | jq '[.[] | select(.brief_role == "" or .brief_role == null)] | length'  # thiếu brief_role con số
cat output/{novel}/meta/cast_ledger.json | jq '[.[] | select(.appearance_count >= 3)] | length'   # Xuất hiện thường xuyên (≥3 Tính thường xuyên
cat output/{novel}/meta/cast_ledger.json | jq 'sort_by(-.appearance_count) | .[:10]'  # Chơi nhiều nhất 10 cá nhân
```

### Phán quyết về sức khỏe

| Kích thước | Sức khỏe | Bất thường | Đối phó |
|---|---|---|---|
| **Số mục so với số chương đã hoàn thành** | Số mục sổ cái ≈ Số chương đã hoàn thành × 0,3-0,6 | > Số chương × 0,8 (Nhân vật xuyên cảnh được nhập sai vào sách) | Kiểm tra xem phần `cast_intros` của writer.md có đủ rõ ràng không |
| **Tỷ lệ lấp đầy_vai trò ngắn gọn** | Thiếu < 30% | Mất tích > 50% | Người viết còn thiếu sót nghiêm trọng - chưa kịp thời hướng dẫn |
| **Sự giống nhau về cùng tên** | Không nghi ngờ cùng một người có nhiều tên | "Lý
| **Nhân vật thường xuyên xuất hiện** | Các mục của `appearance_count >= 5` rất thưa thớt | Một số lượng lớn các mục xuất hiện thường xuyên trên các cung | Điều này được xem xét để nâng cấp lên tệp lõi (Đoạn nâng cấp giai đoạn 3) |
| **Việc thu hồi có được thực hiện hay không** | Khi Writer ghi vào ký tự cũ, trường ký tự của commit_chapter chứa tên hiện có của sổ cái | Nhà văn liên tục bịa ra cái tên giống nhau ("Chu Chu A" và "Chu B" xuất hiện) | việc thu hồi về_cast gần đây không được sử dụng - hãy kiểm tra phần writer.md "Tính liên tục của vai trò hỗ trợ" |

### Xác minh luồng dữ liệu (từ đầu đến cuối)

Sau khi chạy được 5 chương:
1. `cat meta/cast_ledger.json` không được để trống (trừ khi chỉ sử dụng các ký tự cốt lõi trong mỗi chương)
2. Nếu Người viết giới thiệu “Lão Chu” ở Chương 1:
   - Cần có mục nhập cho `Lão Châu` trong `cast_ledger`, `appearance_count=1`
3. Nếu Lão Châu được viết lại ở Chương 5:
   - `Lão Châu.appearance_count=2`, `last_seen_chapter=5`
4. Giá trị trả về của tiểu thuyết_context trong Chương 5 trong `meta/sessions/agents/writer-*.jsonl` sẽ được thấy trong `episodic_memory.recent_cast`.
5. Nếu bạn đã nhìn thấy nó ở bước trước nhưng Người viết không sử dụng nó (chữ Lão Chu không khớp với Chương 1) - đây là một vấn đề cấp bách

### Hiện tại không có chẩn đoán tự động (nhưng ảnh chụp nhanh đã được tải)

`diag.Snapshot.CastLedger` đã được đọc trong `Load()` và có thể được các quy tắc sử dụng trực tiếp - nhưng chưa có quy tắc nào được viết. Việc xác minh vẫn dựa vào việc kiểm tra thủ công bằng lệnh `jq` ở trên.

Nếu bạn muốn thêm quy tắc chẩn đoán (ứng viên) sau:
- `CastBriefRoleMissing`: Tỷ lệ thiếu cảnh báo > 50%
- `CastBloat`: Số mục > Số chương × 0,8 báo động
- `CastPromotionCandidate`: số lượng ngoại hình ≥ 5 và xuyên cung → Đề xuất nâng cấp

Đừng đặt ngưỡng ngay bây giờ—hãy đợi cho đến khi dữ liệu dạng dài xuất hiện và xem xét mức phân bổ thực tế trước khi đặt ngưỡng. Bản thân mã quy tắc chỉ mất 30-50 dòng.

---

## 5. Writer có hoạt động như mong đợi không?

Điều quan trọng nhất cần lo lắng khi viết một câu chuyện dài là **Người viết có thực sự làm theo lời nhắc** không? Quan sát trực tiếp nhất là nhật ký phiên:

```bash
ls output/{novel}/meta/sessions/agents/    # Một bản sao cho mỗi đại lý phụ jsonl
tail -50 output/{novel}/meta/sessions/agents/writer-*.jsonl
```

Hãy xem xét một vài hành vi cụ thể:

| Hành vi mong muốn | Đại diện trong jsonl |
|---|---|
| Người viết đã xem gần đây_cast | Trường `episodic_memory.recent_cast` trong giá trị trả về của công cụ Novel_context không trống |
| Writer điền vào cast_intros trong commit_chapter | `cast_intros` tham số tool_call không trống (chỉ trong các chương có ký tự mới được giới thiệu) |
| Người viết đã sử dụng các đề xuất chương có liên quan | Số lần gọi `read_chapter` > 1 (mặc định 1 lần, nếu vượt quá sẽ xem lại hướng dẫn) |
| Người viết không vi phạm trật tự công cụ | chuỗi tool_call nghiêm ngặt `novel_context → read_chapter → plan_chapter → draft_chapter → check_consistency → commit_chapter` |

Nếu bạn thấy trong jsonl Writer điều chỉnh tiểu thuyết_context hoặc commit_chapter nhiều lần rồi điều chỉnh các công cụ khác - lời nhắc không bị chặn.

---

## 6. Cảnh chạy đường dài có vạch đỏ

Khi đang đọc một câu chuyện dài hơn 100 chương, bạn nên dừng lại và kiểm tra xem có bất kỳ điều nào sau đây xảy ra không:

- [ ] CompassDrift đánh và tồn tại trong 2 cung mà không bị loại
- [ ] cast_ledger Số mục > Số chương đã hoàn thành × 0,8
- Tỷ lệ lấp đầy của Brief_role trong [ ] cast_ledger < 30%
- [ ] Có nhiều nghi phạm cho cùng một nhân vật ("Lão Lý" / "Lý chủ tiệm" cùng tồn tại)
- [ ] Người viết không đọc các ký tự cũ đã có trong near_cast khi viết chương mới (sáng tạo lại)
- [ ] Điều hòa tiểu thuyết_bối cảnh xuất hiện ≥ 5 lần liên tiếp trong phiên Điều phối viên
- [ ] `meta/checkpoints.jsonl` không tương ứng với bước `commit_chapter` sau bất kỳ chương nào được chuyển giao

4 mục đầu tiên là sức khỏe của cơ chế mới này; 3 mục cuối cùng là sự ổn định của cơ chế hiện có.

---

## 7. Thông số bảo trì tài liệu

**Khi thêm một tạo phẩm lớp thực tế (tạo `meta/*.json`/`meta/*.jsonl` mới), đồng bộ hóa:**

1. Thêm truy vấn nhanh vào §2 của tài liệu này
2. Nếu phôi cần được quan sát đặc biệt (không phải là phán đoán "tồn tại/vắng mặt" đơn giản), hãy thêm §X đoạn đặc biệt
3. Nếu bạn muốn chẩn đoán tự động, hãy tải nó vào `internal/diag/snapshot.go::Load` và thêm quy tắc vào `internal/diag/rules_*.go`

**không muốn:**
- Không sao chép tất cả các quy tắc trong `internal/diag/` vào tài liệu này (đó là tài liệu tham khảo về quy tắc, không phải là sổ tay quan sát)
- Không viết quy tắc chẩn đoán cho từng cơ chế - ngưỡng sẽ sai bằng cách vỗ nhẹ vào đầu, quan sát trước rồi bù đắp.
