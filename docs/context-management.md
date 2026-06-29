# Hướng dẫn quản lý bối cảnh

Tài liệu này mô tả hệ thống quản lý bối cảnh hiện tại của `ainovel-cli`, bao gồm:

- Tại sao chúng ta cần quản lý bối cảnh?
- Bối cảnh đến từ đâu?
- Cách nén, khôi phục và chuyển giao trong thời gian chạy
- Giá trị, điều kiện kích hoạt và kịch bản áp dụng của từng chiến lược
- Nơi cần tìm đầu tiên khi có sự cố xảy ra

Mục đích không phải là giới thiệu các khái niệm trừu tượng mà là cho phép những người bảo trì tiếp theo nhanh chóng hiểu được lối vào triển khai và khắc phục sự cố hiện tại bằng cách mở tài liệu này.

## 1. Mục tiêu thiết kế

Việc quản lý bối cảnh của dự án này không dành cho các kịch bản trò chuyện chung mà dành cho các kịch bản tạo mới. Nó cần phải giải quyết một số loại vấn đề cùng một lúc:

1. Các cuộc hội thoại dài sẽ vượt quá cửa sổ ngữ cảnh của mô hình.
2. Thứ cần được bảo tồn trong sáng tạo tiểu thuyết không phải là “bản thân lịch sử trò chuyện”, mà là trí nhớ tường thuật có cấu trúc.
3. Người viết không thể mất trạng thái ký tự, điềm báo, sơ đồ chương, ràng buộc về văn phong và các mục xem lại sẽ được sửa đổi sau khi nén.
4. Khi tiếp tục viết, bạn không thể cho rằng mô hình vẫn "nhớ những gì chúng ta đã nói trước đây" và trước tiên phải dựa vào các hiện vật liên tục.

Vì vậy, chúng tôi áp dụng giải pháp "bộ nhớ theo lớp":

- Bộ nhớ ngắn hạn: phần đuôi của tin nhắn được lưu giữ gần đây nhất
- Bộ nhớ trung hạn: nén tạo `ContextSummary`
- Trí nhớ dài hạn: các tạo phẩm có cấu trúc trong kho dự án
- Khôi phục bộ nhớ: handoff/restore pack/novel_context

## 2. Kiến trúc tổng thể

### 2.1 Các lớp chính

Việc quản lý bối cảnh hiện tại được chia thành bốn lớp:

1. `agentcore/context`
   Chịu trách nhiệm về ngân sách bối cảnh chung, quy trình chính sách, khung nén/khôi phục.

2. `internal/tools/novel_context`
   Chịu trách nhiệm tập hợp dữ liệu có cấu trúc trong dự án mới thành bối cảnh có thể sử dụng được cho vòng hiện tại.

3. `internal/orchestrator/store_summary_*`
   Chịu trách nhiệm nén nhanh dựa trên cửa hàng dành riêng cho người viết.

4. `internal/orchestrator/writer_restore.go`
   Chịu trách nhiệm gắn thêm gói khôi phục nén sau `FullSummary` để đảm bảo Writer có thể tiếp tục viết.

### 2.2 Luồng dữ liệu

Có hai đường dẫn ngữ cảnh chính khi chạy:

1. Đường làm việc bình thường
   - Đại lý gọi `novel_context`
   - `novel_context` đọc tóm tắt chương, kế hoạch, vai trò, dòng thời gian và các dữ liệu khác từ cửa hàng
   - Những dữ liệu này nhập dấu nhắc vòng hiện tại

2. Đường dẫn ngữ cảnh quá dài
   - `ContextManager` phát hiện áp lực mã thông báo
   - Nén theo thứ tự chiến lược
   - Ưu tiên nén nhẹ và nén dựa trên cửa hàng
   - Chỉ đi nếu chưa đủ LLM `FullSummary`
   - Tiêm gói khôi phục sau `FullSummary`

## 3. Tệp chính

### 3.1 Công cụ bối cảnh phổ quát

- `../agentcore/context/strategy.go`
- `../agentcore/context/engine.go`
- `../agentcore/context/strategy_tool.go`
- `../agentcore/context/strategy_trim.go`
- `../agentcore/context/strategy_summary.go`
- `../agentcore/context/message.go`
- `../agentcore/context/summary_run.go`

tác dụng:

- Xác định `Strategy`/`ForceCompactionStrategy`
- Chịu trách nhiệm thực hiện chuỗi chiến lược dựa trên ngân sách
- Chịu trách nhiệm về đại diện `ContextSummary` và chuyển đổi LLM
- Chịu trách nhiệm nén thông báo LLM của `FullSummary`

### 3.2 Đi dây phía dự án

- `internal/orchestrator/agents.go`

tác dụng:

- Biên tập viên/Điều phối viên `ContextManager`
- Tiêm thêm `StoreSummaryCompact` vào Writer
- Định cấu hình lời nhắc `FullSummary` tùy chỉnh mới cho Trình ghi
- Cấu hình `writerRestorePack` cho Writer

### 3.3 Nén và phục hồi phía dự án

- `internal/orchestrator/store_summary_strategy.go`
- `internal/orchestrator/store_summary_builder.go`
- `internal/orchestrator/writer_restore.go`

tác dụng:

- Ưu tiên sử dụng dữ liệu lưu trữ để nén nhanh trước khi phân tích LLM
- Thống nhất bối cảnh có cấu trúc cần thiết cho việc nén và khôi phục Writer
- Thêm thông báo khôi phục bộ nhớ thuần túy sau `FullSummary`

### 3.4 Tập hợp ngữ cảnh có cấu trúc

- `internal/tools/novel_context.go`
- `internal/tools/novel_context_builders.go`
- `internal/domain/runtime.go`

tác dụng:

- Xác định `ContextProfile`/`MemoryPolicy`
- Quyết định tải bao nhiêu bản tóm tắt chương, bao nhiêu mốc thời gian và có bật tính năng tóm tắt phân cấp hay không
- Tập hợp các chương, nhân vật, điềm báo, dòng thời gian, review kinh nghiệm,.. trong cửa hàng

### 3.5 Chuyển giao và phục hồi

- `internal/orchestrator/handoff_policy.go`
- `internal/orchestrator/recovery_engine.go`

tác dụng:

- Ưu tiên dựa vào bàn giao trong giai đoạn tính năng/làm lại/đánh giá
- Đưa gói chuyển giao có cấu trúc vào dấu nhắc trong quá trình khôi phục

### 3.6 Khả năng quan sát

- `internal/orchestrator/run.go`
- `internal/orchestrator/runtime.go`
- `internal/entry/tui/panels.go`

tác dụng:

- Ghi nhật ký các sự kiện viết lại bối cảnh
- Tên chính sách đầu ra, thay đổi mã thông báo và số lượng lưu giữ tin nhắn
- Cho phép TUI xem context hiện tại là `projected` hay `compacted`

## 4. ContextManager được lắp ráp như thế nào?

Cả Người viết và Người điều phối đều sử dụng `newContextManager` nhưng cấu hình thì khác nhau.

Các thông số chính hiện tại của `contextManagerConfig`:

- `ContextWindow`
  Mô hình tổng cửa sổ ngữ cảnh.

- `ReserveTokens`
  Mã thông báo dành riêng cho đầu ra của mô hình.

- `KeepRecentTokens`
  Ngân sách đuôi tin nhắn gần đây nhất cần giữ nguyên khi nén.

- `ToolMicrocompact`
  Công cụ cấu hình nén vi mô.

- `ExtraStrategies`
  Chiến lược nén bổ sung phía dự án. Hiện tại Writer được dùng để mount `StoreSummaryCompact`.

- `Summary`
  Cấu hình `FullSummary`, bao gồm lời nhắc tùy chỉnh và hook sau tóm tắt.

Giá trị cấu hình thực tế hiện tại:

| Thông số | Nhà văn | Điều phối viên |
|------|--------|-------------|
| ReserveTokens | 16,384 | 32,000 |
| KeepRecentTokens | 20,000 | 30,000 |
| CommitOnProject | false | true |
| Ngưỡng nhàn rỗi | 5 phút | Không có |
| Chiến lược bổ sung | Cửa hàngTóm tắtCompact | Không có |
| Lời nhắc tóm tắt tùy chỉnh | Phiên Bản Truyện Tiểu Thuyết | Mặc định (Phiên bản hỗ trợ mã) |

Ngưỡng kích hoạt nén = `ContextWindow - ReserveTokens`. Ví dụ: khi cửa sổ là 128K, Trình ghi sẽ kích hoạt ở mức ~112K và Bộ điều phối sẽ kích hoạt ở mức ~96K.

Trình tự quy trình chiến lược hiện tại của Writer là:

1. `ToolResultMicrocompact`
2. `LightTrim`
3. `StoreSummaryCompact`
4. `FullSummary`

Trình tự này có ý nghĩa rõ ràng:

- Trước tiên hãy sử dụng cách rẻ nhất để loại bỏ tiếng ồn của công cụ
- Cắt lại các khối văn bản dài thêm
- Nếu dữ liệu lưu trữ đủ, trực tiếp thực hiện nén có cấu trúc với LLM bằng 0
- Chỉ thoát ra phần tóm tắt LLM ở cuối

## 5. Vai trò của từng chiến lược

### 5.1 ToolResultMicrocompact

Địa điểm thực hiện:

- `../agentcore/context/strategy_tool.go`

tác dụng:

- Làm sạch lịch sử `tool_result`
- Thay thế kết quả công cụ cũ bằng văn bản giữ chỗ ngắn

giá trị:

- Nội dung được các công cụ trả về thường có kích thước lớn và mật độ thông tin thấp.
- Rất nhiều công cụ cũ hóa ra chỉ là "tiếng ồn xử lý", không phải là ký ức mới lạ

Tính năng cấu hình Writer hiện tại:

- `IdleThreshold = 5m` được thiết lập

Điều này có nghĩa là:

- Nếu các tin nhắn trợ lý gần đây không hoạt động vượt quá ngưỡng
- Sẽ giảm mạnh hơn số lượng kết quả dụng cụ cũ được giữ lại

Các tình huống áp dụng:

- Nhiều vòng `novel_context`
- Sau nhiều vòng công cụ đọc/kiểm tra/nháp

### 5.2 LightTrim

Địa điểm thực hiện:

- `../agentcore/context/strategy_trim.go`

tác dụng:

- Cắt bớt các đoạn văn bản quá dài
- Giữ nguyên phần đầu và đuôi, thay thế bằng phần giữ chỗ ở giữa

giá trị:

- Giữ nguyên cấu trúc tin nhắn
- chi phí thấp
- Rất thích hợp để xử lý văn bản gốc của các chương rất dài hoặc các đoạn đầu ra có kích thước lớn

Các tình huống áp dụng:

- Một tin nhắn thì quá dài nhưng toàn bộ lịch sử thì không cần tóm tắt

### 5.3 StoreSummaryCompact

Địa điểm thực hiện:

- `internal/orchestrator/store_summary_strategy.go`
- `internal/orchestrator/store_summary_builder.go`

tác dụng:

- Khi bối cảnh Writer quá dài
- Ưu tiên sử dụng bộ nhớ có cấu trúc trong Persistence Store để thay thế các tin nhắn cũ
- LLM không được gọi

Đây không phải là bản tóm tắt cuộc trò chuyện mà là "sự thay thế bộ nhớ có cấu trúc".

Dữ liệu cốt lõi hiện được giữ lại bao gồm:

- Tiến độ hiện tại
- Tóm tắt các chương gần đây
- Kế hoạch chương hiện tại
- Đề cương chương hiện tại
- Tóm tắt hồ quang hiện tại
- Tổng hợp khối lượng hiện tại
- Ảnh chụp nhân vật
- Điềm báo chủ động
- Xem xét những vấn đề cần sửa đổi
- Dòng thời gian gần đây
- Quy tắc phong cách

Điều kiện tiên quyết kích hoạt:

- Chương hiện tại lớn hơn 1
- Đã có đủ tóm tắt lịch sử trong cửa hàng
- Và chương hiện tại có ít nhất dữ liệu trạng thái làm việc
  - `chapter_plan` hoặc `current_outline`

giá trị:

- Giảm thời gian nén LLM
- Ngăn chặn thông tin quan trọng trôi dạt trong phần tóm tắt
- Làm cho trí nhớ dài hạn dựa vào sự kiện giao dịch trước tiên thay vì lịch sử trò chuyện

Tại sao chỉ dành cho Writer:

- Đây là chiến lược kinh doanh mới lạ, không phải là chiến lược khung chung
- Chế độ ngữ cảnh của Điều phối viên / Biên tập viên khác
- Đầu tiên, hợp lý nhất là xác minh trên Writer cần bộ nhớ tạo liên tục nhất.

### 5.4 FullSummary

Địa điểm thực hiện:

- `../agentcore/context/strategy_summary.go`
- `../agentcore/context/summary_run.go`

tác dụng:

- Sử dụng tạo mô hình `ContextSummary` khi các lớp trên không đủ
- Giữ phần cuối của tin nhắn gần đây
- Biến bối cảnh trước đó thành điểm kiểm tra có cấu trúc

Writer khác với trợ lý mã mặc định:

- Người viết sử dụng lời nhắc tóm tắt tùy chỉnh
- Nội dung của phần tóm tắt có yêu cầu ghi nhớ rõ ràng:
  - Tiến độ hiện tại
  - Trạng thái thời gian thực của nhân vật
  - Hoạt động báo trước và manh mối
  - Xem xét các ý kiến ​​phản hồi và những vấn đề cần chỉnh sửa
  - Phong cách và nhịp điệu
  - Các quyết định quan trọng
  - Bước tiếp theo
  - Bối cảnh chính

giá trị:

- là chiến lược che đậy tối thượng
- Ngay cả khi dữ liệu lưu trữ không đủ, tính liên tục vẫn có thể được duy trì thông qua LLM

### 5.5 Bộ Ngắt Mạch

Địa điểm thực hiện:

- `../agentcore/context/engine.go`

tác dụng:

- Khi lỗi nén liên tiếp đạt tới ngưỡng (mặc định là 3 lần), bỏ qua đợt nén hiện tại
- vẫn phát ra `RewriteEvent` (`Reason = “circuit_breaker”`) khi bỏ qua
- TUI sẽ hiển thị phạm vi là "mạch bị bỏ qua"
- Sử dụng chế độ nửa mở: sau khi bỏ qua một vòng, lần sau sẽ thử lại, đặt lại nếu thành công và bỏ qua lần nữa nếu thất bại.

Tại sao bạn cần:

- Tóm tắt LLM có thể bị lỗi liên tục do mạng, mô hình bị từ chối, v.v.
- Nếu không có cầu dao, mỗi vòng Dự án sẽ thử và thất bại, gây lãng phí lệnh gọi API.
- Chất thải này tích tụ qua những buổi viết dài

Khắc phục sự cố:

- Nếu TUI tiếp tục hiển thị "Bỏ qua mạch", có vấn đề với đường dẫn tóm tắt LLM
- Kiểm tra slog cho các sự kiện viết lại ngữ cảnh cho `reason=circuit_breaker`
- Thổi không ảnh hưởng đến `StoreSummaryCompact` (không điều chỉnh LLM)

### 5.6 Ước tính mã thông báo (nhận thức của CJK)

Địa điểm thực hiện:

- `../agentcore/context/usage.go`

tác dụng:

- Tất cả thời gian kích hoạt nén và kiểm soát ngân sách đều dựa vào ước tính mã thông báo
- `estimateTextTokens` tự động phát hiện xem văn bản có bị chi phối bởi các ký tự CJK hay không
- Văn bản chính của CJK: `runes × 1.5`
- Văn bản chiếm ưu thế ASCII: `bytes / 4`

Tại sao bạn không thể sử dụng `bytes/4` tiêu chuẩn:

- UTF-8 tiếng Trung một ký tự = 3 byte
- `bytes/4` sẽ ước tính một ký tự tiếng Trung là 0,75 token, thực tế là khoảng 1,5 token
- Đánh giá thấp theo hệ số 2 gây ra độ trễ nghiêm trọng trong quá trình kích hoạt nén

Phạm vi ảnh hưởng:

- `EstimateTokens` (tin nhắn đơn)
- `EstimateTotal` (danh sách tin nhắn)
- `EstimateContextTokens` (ước tính kết hợp: LLM báo cáo Cách sử dụng + ước tính thông báo đuôi)
- Cắt giảm ngân sách trong `store_summary_builder.go`

Lưu ý: Các đối số của ToolCall là JSON (ASCII chiếm ưu thế), vẫn sử dụng `bytes/4` và không bị ảnh hưởng bởi các điều chỉnh của CJK.

## 6. Tại sao Writer lại có hai bộ "bộ nhớ nén"?

Hiện tại Writer có 2 link trông giống nhau nhưng có trách nhiệm khác nhau:

### 6.1 StoreSummaryCompact

Trách nhiệm:

- Thay thế tin nhắn cũ trực tiếp trong quá trình nén

Đặc trưng:

- Xảy ra trước `FullSummary`
- Không LLM
- Thay thế lịch sử trước đó bằng cửa hàng

### 6.2 writerRestorePack

Địa điểm thực hiện:

- `internal/orchestrator/writer_restore.go`

Trách nhiệm:

- Thêm thông báo khôi phục sau `FullSummary`

Đặc trưng:

- xảy ra sau khi nén LLM
- Tiêm qua `PostSummaryHook`
- Dùng để bổ sung cho Writer những thông tin có cấu trúc cần phải xem khi tiếp tục tạo

Tại sao bạn cần cả hai:

- `StoreSummaryCompact` không phải lúc nào cũng trúng
  - Ví dụ: khi chương đầu tiên hoặc dữ liệu lưu trữ không đủ
- `FullSummary` Dù bạn làm tốt đến đâu, bạn cũng có thể bỏ lỡ thông tin chính xác trong cửa hàng
- Vì vậy gói khôi phục đóng vai trò là phương sách cuối cùng

Bây giờ cả hai đã chia sẻ `store_summary_builder.go` để tránh bị lệch cỡ nòng.

## 7. Vai trò của tiểu thuyết_bối cảnh

Địa điểm thực hiện:

- `internal/tools/novel_context.go`
- `internal/tools/novel_context_builders.go`

`novel_context` không phải là một chiến lược nén, nó là một "trình biên dịch ngữ cảnh có cấu trúc" thời gian chạy.

Nó chia dữ liệu trong cửa hàng thành nhiều loại:

- `working_memory`
  - Kế hoạch chương hiện tại
  - Đề cương chương hiện tại
  - Tóm tắt các chương gần đây
  - Dòng thời gian
  - checkpoint
  - previous tail

- `episodic_memory`
  - Trạng thái nhân vật
  - Tình trạng mối quan hệ
  - Thay đổi trạng thái gần đây
  - điềm báo

- `reference_pack`
  - Cài đặt và dữ liệu tham khảo ổn định hơn

- `selected_memory`
  - Một số lượng nhỏ ký ức quan trọng được chọn theo nhiệm vụ hiện tại

giá trị:

- Nó xác định bối cảnh mới có cấu trúc thực sự được "đưa vào mô hình" cho mỗi vòng
- `StoreSummaryCompact` không tự gọi chính nó mà sử dụng lại cùng nguồn dữ liệu và lắp ráp ý tưởng với nó

## 8. ContextProfile và MemoryPolicy

Địa điểm thực hiện:

- `internal/domain/runtime.go`

### 8.1 ContextProfile

tác dụng:

- Xác định kích thước cửa sổ tải dựa trên tổng số chương

Quy định hiện hành:

- Chương `<= 15`
  - Tóm tắt các chương `10` gần đây
  - Dòng thời gian chương `10` gần đây

- Chương `<= 50`
  - Tóm tắt các chương `5` gần đây
  - Dòng thời gian chương `8` gần đây

- Chương `> 50`
  - Tóm tắt các chương `3` gần đây
  - Dòng thời gian chương `5` gần đây
  - Kích hoạt tính năng tóm tắt phân cấp

giá trị:

- Kiểm soát kích thước bối cảnh
- Tránh nhồi nhét toàn bộ lịch sử vào gợi ý khi viết truyện dài

### 8.2 MemoryPolicy

tác dụng:

- Ghi rõ ràng chính sách sử dụng ngữ cảnh hiện tại
- cho đầu ra `novel_context`
- Để sử dụng bằng logic chuyển giao/nhắc nhở/chẩn đoán

Các trường chính:

- `SummaryWindow`
- `TimelineWindow`
- `LayeredSummaries`
- `SummaryStrategy`
- `HandoffPreferred`
- `ReadOnlyThreshold`

giá trị:

- Thay đổi "cách hệ thống hiện tại nên sử dụng bộ nhớ" từ logic ngầm sang chính sách thời gian chạy rõ ràng

## 9. Vai trò của bàn giao

Địa điểm thực hiện:

- `internal/orchestrator/handoff_policy.go`

Khi công việc chuyển sang các giai đoạn dài hơn, phức tạp hơn và phụ thuộc nhiều hơn vào các tạo phẩm có cấu trúc, hệ thống sẽ ưu tiên chuyển giao.

gói chuyển giao sẽ ghi lại:

- Giai đoạn hiện tại và dòng chảy
- Vị trí chương tiếp theo
- Bài nộp gần đây
- Đã xem xét gần đây
- tóm tắt gần đây
- chính sách bộ nhớ hiện tại
- Hướng dẫn khôi phục

giá trị:

- Không dựa vào lịch sử trò chuyện khi tiếp tục sau khi bị gián đoạn
- Ưu tiên dựa vào các tạo phẩm có cấu trúc trong các kịch bản làm lại, đánh giá và dạng dài

## 10. Khả năng quan sát và xử lý sự cố

### 10.1 Sự kiện viết lại ngữ cảnh

Địa điểm thực hiện:

- `internal/orchestrator/run.go`

Mỗi lần viết lại ngữ cảnh được xuất ra thông qua `contextRewriteCallback`:

- `reason`
- `strategy`
- `committed`
- `tokens_before`
- `tokens_after`
- `messages_before`
- `messages_after`
- `compacted_count`
- `kept_count`
- `split_turn`
- `incremental`
- `summary_runes`
- `duration_ms`

Điều này sẽ đồng thời nhập:

- `slog`
- hàng đợi ranh giới thời gian chạy
- Sự kiện TUI `COMPACT`

### 10.2 Những gì nhìn thấy trong TUI

TUI sẽ hiển thị:

- Mã thông báo bối cảnh hiện tại (có màu gradient sức khỏe)
- context window
- Phạm vi bối cảnh hiện tại (bao gồm cả "bỏ qua mạch")
- Tên chính sách cuối cùng hiện tại
- số lượng tóm tắt

Ý nghĩa màu sắc của phần trăm ngữ cảnh (được triển khai trong `internal/entry/tui/layout.go`):

| Màu sắc | Tình trạng | Ý nghĩa |
|------|------|------|
| Xanh | < 70% | Hào phóng, xa ngưỡng nén |
| Vàng | 70-85% | Gần đến ngưỡng nén |
| Đỏ | > 85% | Sắp hoặc đang bị nén |

Nhãn tiếng Trung của Scope:

| Phạm vi | Hiển thị | Ý nghĩa |
|-------|------|------|
| đường cơ sở | đường cơ sở | trạng thái bình thường |
| dự kiến ​​| chiếu | xem trước nén tạm thời |
| nén | Đã gửi | Nén đã có hiệu lực |
| đã phục hồi | phục hồi | phục hồi sau khi tràn |
| bỏ qua | cầu dao bị bỏ qua | quá trình nén bị bỏ qua bởi bộ ngắt mạch |

giá trị:

- Có khả năng xác định nhanh chóng tình trạng của bối cảnh hiện tại
- Màu vàng/đỏ khi bạn có thể dự kiến ​​sẽ xảy ra hiện tượng nén
- Thấy "mạch bị bỏ qua" cho biết có vấn đề với đường dẫn phân loại LLM

### 10.3 Cần tìm ở đâu đầu tiên khi có vấn đề

#### Tình huống 1: Người viết mất sơ đồ chương sau khi nén

Hãy xem trước:

- `novel_context` có được đưa vào `chapter_plan` ổn định hay không
- `store_summary_builder.go` có nên lấy `chapterPlan` không
- `writerRestorePack` có được làm mới hay không

Tài liệu chính:

- `internal/tools/novel_context_builders.go`
- `internal/orchestrator/store_summary_builder.go`
- `internal/orchestrator/session.go`

#### Tình huống 2: Mất trạng thái ký tự/điềm báo sau khi nén

Hãy xem trước:

- `LoadLatestSnapshots`
- `LoadActiveForeshadow`
- `store_summary_builder.go`
-Liệu lời nhắc tóm tắt của Writer có bị ghi đè hay không

#### Tình huống 3: Nén thường xuyên nhưng luôn thiếu store_summary

Hãy xem trước:

- Chương hiện tại có phải là `<= 1` không?
- Đã có bản tóm tắt / phần tóm tắt / tập gần đây chưa
- `chapter_plan` hay `current_outline` tồn tại
- `writer.Context.Strategy` cuối cùng có được ghi là `full_summary` không?

#### Tình huống 4: Thiếu ngữ cảnh sau khi khôi phục

Hãy xem trước:

- Liệu chuyển giao có được tạo ra hay không
-Restore gói có làm mới không
- nhắc nhở khôi phục xem có nên tiêm chuyển giao hay không

#### Tình huống 5: Quá nhiều kết quả công cụ dẫn đến bối rối

Hãy xem trước:

- `ToolResultMicrocompact` có trúng không
- `IdleThreshold` có hiệu quả không?

## 11. Sự đánh đổi trong thực hiện hiện nay

### Phương hướng tuân thủ đã được xác định rõ ràng

1. Đừng nhồi nhét logic nghiệp vụ mới vào `agentcore`
2. Ưu tiên dựa vào các cửa hàng có cấu trúc hơn là lịch sử trò chuyện
3. Người viết sử dụng lời nhắc tóm tắt tiểu thuyết đặc biệt
4. Nén và phục hồi nên chia sẻ cùng một trình tạo càng nhiều càng tốt để tránh trôi cỡ nòng.

### Những hạn chế hiện vẫn được cố ý giữ lại

1. `StoreSummaryCompact` chỉ dành cho Writer
2. Chương 1 sẽ không có bản thu gọn dựa trên cửa hàng
3. Khi dữ liệu lưu trữ không đủ, nó vẫn quay trở lại `FullSummary`.
4. `writerRestorePack` là phần bù bổ sung và không thay thế `FullSummary`.

Những hạn chế này không phải là khiếm khuyết mà là những ranh giới được đặt ra ở giai đoạn hiện tại để kiểm soát sự phức tạp.

## 12. Tóm tắt một câu

Việc quản lý bối cảnh của dự án này không đơn giản như việc "rút ngắn những cuộc trò chuyện dài dòng", mà là:

`Ưu tiên sử dụng bộ nhớ tiểu thuyết có cấu trúc để duy trì tính liên tục và chỉ sử dụng bộ nhớ tiểu thuyết có cấu trúc khi cần thiết. LLM Chuyển đến đoạn hội thoại tóm tắt; và cố gắng dựa vào cùng một tập hợp các tạo phẩm bền vững trong ba liên kết nén, phục hồi và chuyển giao.`

Nếu bạn muốn thay đổi hệ thống này trong tương lai, hãy ưu tiên ba điều sau:

1. Đừng để những ký ức quan trọng của Nhà văn chỉ dựa vào lịch sử trò chuyện nữa.
2. Đừng để cỡ nòng `store_summary` và `writer_restore` phân đôi.
3. Khi xảy ra sự cố liên tục, trước tiên hãy kiểm tra xem tạo phẩm có cấu trúc đã vào ngữ cảnh hay chưa, sau đó quyết định xem có thay đổi lời nhắc hay không.
