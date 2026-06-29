# kiến ​​trúc thời gian chạy ainovel-cli

> Hãy để LLM viết xong một cuốn tiểu thuyết trong một lần. Máy chủ chỉ thực hiện khởi động/khôi phục/định tuyến/quan sát và quyền ra quyết định được giao cho mô hình nhiều nhất có thể.

---

## 1. Mục tiêu (theo mức độ ưu tiên)

1. **Tính ổn định**: nhập một câu và viết ổn định toàn bộ tiểu thuyết (200 ~ 500 chương). Sẽ không có sự gián đoạn ở giữa do vấn đề kiến ​​trúc.
2. **Có thể lặp lại chất lượng**: lời nhắc/tài liệu tham khảo/thứ nguyên đánh giá/chiến lược bối cảnh có thể được điều chỉnh độc lập mà không liên quan đến kiến ​​trúc.
3. **Có thể phục hồi**: Có thể tiếp tục từ điểm kiểm tra mới nhất sau sự cố, ngắt kết nối hoặc tạm dừng.
4. **Có thể quan sát**: Có thể kiểm tra tiến độ, sản phẩm và thời gian thực hiện từng bước trong mỗi chương.

“Ổn định” là tiền đề, còn “chất lượng” là cấp trên. Ưu tiên sự ổn định của dịch vụ với mọi quyết định về kiến ​​trúc.

---

## 2. Nguyên tắc cốt lõi

### 2.1 Tạo và điều khiển ổ đĩa LLM, định tuyến quy trình ổ đĩa máy chủ

Không gian ra quyết định của tác nhân dọc bị đóng: lưu đồ cố định, các nhánh bị giới hạn và dựa trên thực tế. Hai loại quyết định có các vectơ khác nhau:

- **Sáng tạo và xét xử** (Ngữ nghĩa/Chất lượng/Hiểu ý định) → LLM. Khả năng quản lý của Người viết/Biên tập viên/Kiến trúc sư/Điều phối viên được hưởng lợi tuyến tính khi nâng cấp mô hình
- **Định tuyến quy trình** (Đọc bảng tra cứu sự kiện) → Mã. Hàm thuần `flow.Router` + kiểm tra đơn, tỷ lệ lỗi tiến tới 0

Máy chủ không gọi trực tiếp SubAgent. Thay vào đó, Bộ định tuyến luồng sẽ tính toán các hướng dẫn tại ranh giới đồng bộ hóa được công cụ `subagent`/`reopen_book` của Điều phối viên trả về thành công và đưa hướng dẫn đó vào vòng tiếp theo của quá trình chạy hiện tại qua `coordinator.Steer("[Host Ra lệnh]…")`. `FollowUp` chỉ bị cạn kiệt sau khi tác nhân tự nhiên không hoạt động và không thể thực hiện định tuyến quy trình chính.

### 2.2 Công cụ là giao diện duy nhất của lớp dữ kiện

Tất cả các tương tác với hệ thống tệp, Tiến trình và Điểm kiểm tra đều được thực hiện bằng các công cụ. **Công cụ viết phải có bộ ba phần nguyên tử**: vị trí tạo tác + Tiến bộ + Bổ sung điểm kiểm tra, được hoàn thành trong khóa mutex. Chạy lại cùng một công cụ để nhận được kết quả tương tự hoặc bỏ qua nó (thông báo là bình thường).

### 2.3 Lớp quan sát chỉ quan sát

Giao diện người dùng, chẩn đoán, nhật ký sự kiện đều là những ứng dụng thụ động được chiếu từ luồng sự kiện/tạo phẩm chỉ đọc. Đọc dữ kiện, không tạo ra dữ kiện và không ảnh hưởng đến luồng điều khiển.

**`internal/diag` là hệ thống con có khả năng quan sát duy nhất của động cơ** - một cơ sở hỗ trợ hạng nhất, nhưng không phải là cốt lõi của sản phẩm (cốt lõi là công cụ tạo ra §6; tiểu thuyết vẫn có thể được viết mà không cần diag). Nó đọc hầu hết tất cả các tạo phẩm + phiên + nhật ký + điểm kiểm tra và đảm nhận hai vai trò: ① **Chẩn đoán chất lượng sáng tạo** (quy tắc → Tìm kiếm, báo cáo trên màn hình `/diag`); ② **Khắc phục sự cố trong thời gian chạy + xuất không nhạy cảm** (văn bản loại bỏ khung hành vi + tập hợp vòng lặp → lớp phủ `meta/diag-export.md`, để người dùng đăng sự cố; người bảo trì có thể xác định các sự cố vòng lặp/gián đoạn vô hạn ngay cả khi họ không thể nhận được đầu ra cục bộ).

**Kỷ luật quan sát viên (Không nới lỏng)**: diag có thể chẩn đoán và đưa ra đề xuất, nhưng **không bao giờ tự làm** - nó không tự động sửa chữa, không tiếp tục chạy và không thay đổi quy trình. Càng mạnh thì người ta càng muốn "sửa", chúng ta càng phải bám vào điều này, nếu không sẽ rơi lại vào những hố bị xóa như idResume / StallDetector (xem §10.5, §10.14). Khi duy trì các hợp đồng cơ sở hạ tầng cho các cấu trúc bên ngoài (chẳng hạn như `RuntimeCapture`), không được thay đổi các trường theo ý muốn.

### 2.4 Lớp dữ kiện phẳng

Chỉ có ba loại sự thật:

- **Tiến trình** — chỉ mục tiến độ (chương nào đã được viết, danh sách sẽ được viết lại)
- **Điểm kiểm tra** — bản ghi tiến bộ ở cấp độ (kế hoạch / bản nháp / cam kết / đánh giá / arc_summary)
- **Tạo tác** — Văn bản chương, dàn ý, vai trò, tóm tắt và các sản phẩm khác

Không giới thiệu các khái niệm trừu tượng như WorkflowInstance / TaskInstance / Command / Dispatcher.

### 2.5 Ba nguyên tắc sắt

**Quy tắc sắt 1: Công cụ chỉ trả về dữ kiện chứ không trả về hướng dẫn lập lịch chéo**. `commit_chapter` trả về các trường có cấu trúc như `arc_end_reached`/`next_skeleton_arc`; không có chuỗi lệnh giống `[hệ thống]` nào được bao gồm. Trường `next_step` trong tác nhân phụ là hướng dẫn nội tuyến cho một tuyên bố thực tế ("Tôi vừa lưu kế hoạch, bước tiếp theo là bản nháp") và không vi phạm - xem §6.4.

**Quy tắc sắt 2: Bộ định tuyến luồng chịu trách nhiệm định tuyến quy trình**. `Route(state) → *Instruction` của `internal/host/flow/router.go` là một hàm thuần túy; Máy chủ kích hoạt `Dispatch` tại ranh giới đồng bộ hóa của chuỗi thực thi công cụ Điều phối viên và sử dụng `Steer` để đưa `[Host Ra lệnh]` vào vòng đầu vào tiếp theo của lần chạy hiện tại. Trả về con số không có nghĩa là "phân xử kịch bản và để LLM tự chủ". **Kênh lệnh không im lặng**: Khi Route liên tục tính toán cùng một lệnh (cho biết trạng thái chưa được nâng cao kể từ lần gửi trước), Dispatcher đính kèm thông tin thực tế "Vấn đề thứ N" và gửi lại thay vì âm thầm nuốt nó - "kết quả định tuyến trùng lặp" là thông tin mà chỉ Host mới có thể quan sát được. Sự im lặng sẽ khiến Điều phối viên rơi vào mâu thuẫn kép là “không hành động nếu không có lệnh/không dừng lại với StopGuard”. Không có ngưỡng hoặc bộ ngắt mạch và LLM quyết định cách thoát khỏi rắc rối.

**Quy tắc sắt thứ ba: Điều phối viên không thể kết thúc lượt trừ khi Giai đoạn=Hoàn thành**. StopGuard chặn tin nhắn người dùng được chèn `end_turn` ở lớp Agentcore; nó không thể dừng nâng cấp trong 5 lần liên tiếp và chấm dứt. Ba tác nhân phụ (kiến trúc sư/nhà văn/biên tập viên) có `CheckpointDeltaGuard` riêng.

---

## 3. Toàn cảnh kiến ​​trúc

```
[Entry: TUI / headless]
        │ prompt / steer
[Host Vỏ mỏng]
   ├── observer        sự kiện → UI/phép chiếu log
   ├── flow.Dispatcher Đồng bộ hóa ranh giới công cụ → Route(state) → Steer
   └── usage / Quản lý người mẫu
        │
[Coordinator (LLM, MaxTurns=100_000)]
   ├── Phán quyết khi khởi động architect_short / long
   ├── nhận được [Host Ra lệnh] → phát ra subagent tool_call
   └── nhận được [sự can thiệp của người dùng] → quyền tự quyết
        │
[architect / writer / editor SubAgent (độc lập run + context + Người mẫu)]
        │ Cuộc gọi công cụ
[Tools]  novel_context · read_chapter · plan_chapter · draft_chapter · edit_chapter
         check_consistency · commit_chapter · save_review · save_arc_summary
         save_volume_summary · save_foundation
        │ Bộ ba mảnh nguyên tử
[Store: hệ thống tập tin (tmp + rename)]
   Progress · Checkpoints · Outline · Drafts · Summaries · Characters · World · Signals
```

| Lớp | Phải làm gì | Không nên làm gì |
|---|---|---|
| Nhập cảnh | Hiển thị, nhận đầu vào | Quyết định kinh doanh |
| Máy chủ | Khởi động/Phục hồi/Can thiệp/Chiếu sự kiện/Định tuyến luồng | Bỏ qua Điều phối viên và gọi trực tiếp SubAgent; viết trạng thái |
| Điều phối viên | Thực hiện lệnh Host, xác định người dùng Chỉ đạo, khởi động công cụ lập kế hoạch | Quyết định bước tiếp theo của mỗi chương; ghi tập tin |
| Đại lý | Suy nghĩ, viết, đánh giá | Đọc và viết trực tiếp Lưu trữ |
| Công cụ | IO nguyên tử + điểm kiểm tra + bình thường | Hướng dẫn lập kế hoạch cho các đại lý phụ |
| Cửa hàng | Vị trí hệ thống tập tin | Logic kinh doanh |

Phụ thuộc một chiều: `entry → host → agents → tools → store → domain`. `tools/` không tham chiếu `agents/host/` và `host/` không tham chiếu trực tiếp `tools/store/`. Các mô-đun độc lập theo chiều ngang: `errs/` có thể được tham chiếu bởi bất kỳ lớp nào, `diag/` đăng ký luồng sự kiện máy chủ + `store/` chỉ đọc.

---

## 4. Mô hình dữ liệu

### 4.1 Progress（`internal/domain/runtime.go`）

```go
type Progress struct {
    NovelName         string
    Phase             Phase           // init / premise / outline / writing / complete
    CurrentChapter    int
    TotalChapters     int
    CompletedChapters []int
    TotalWordCount    int
    ChapterWordCounts map[int]int
    InProgressChapter int             // Chương đang được viết
    Flow              FlowState       // writing / reviewing / rewriting / polishing / steering
    PendingRewrites   []int
    StrandHistory     []string        // dominant_strand sự liên tiếp
    HookHistory       []string        // hook_type sự liên tiếp
    CurrentVolume, CurrentArc int     // lớp dài
    Layered           bool
}
```

Logic điều khiển chỉ đọc các trường dữ kiện ở trên và không dựa vào bất kỳ "dấu thời gian cập nhật" nào - thông tin thời gian được `OccurredAt` của điểm kiểm tra mang theo.

### 4.2 Checkpoint（`internal/domain/checkpoint.go`）

```go
type Scope      struct { Kind ScopeKind; Chapter, Volume, Arc int }
type Checkpoint struct {
    Seq        int64       // tăng đơn điệu
    Scope      Scope       // chapter / arc / volume / global
    Step       string      // plan / draft / commit / review / arc_summary / ...
    Artifact   string
    Digest     string
    OccurredAt time.Time
}
```

Bộ nhớ: `meta/checkpoints.jsonl`, chỉ nối thêm. Việc ghi lặp lại vào cùng một `Scope+Step+Digest` được coi là bình thường và không tạo ra các hàng mới.

### 4.3 Hiện vật và Tín hiệu

Các thành phần lạ nằm trong `store/outline.go` `drafts.go` `summaries.go` `characters.go` `world.go` - mỗi thành phần lạ có thể được tham chiếu bằng điểm kiểm tra.

Tín hiệu: `PendingCommit` (khôi phục ngắt cam kết) / `PendingSteer` (sự can thiệp của người dùng trong thời gian ngừng hoạt động). Đọc trong khi khởi động/tiếp tục, không phải trong thời gian chạy.

---

## 5. Đặc tính dụng cụ

Công cụ là điểm tương tác duy nhất giữa lớp thực tế và Tác nhân.

### 5.1 Công cụ đọc

`novel_context(scope)` / `read_chapter(n)` - có thể được gọi bất cứ lúc nào, không dựa vào trạng thái trước và trả về đủ dữ liệu để LLM đưa ra quyết định độc lập.

### 5.2 Dụng cụ viết (Bộ ba món Atomic)

Mỗi cuộc gọi thành công phải: đặt tạo tác → Tiến trình → bổ sung điểm kiểm tra. Hoàn thành trong vòng khóa mutex ba bước.

| Công cụ | Cổ vật | Bước |
|---|---|---|
| `plan_chapter` | drafts/chXX.plan.json | plan |
| `draft_chapter` | drafts/chXX.draft.md | draft |
| `edit_chapter` | drafts/chXX.draft.md | edit |
| `check_consistency` | Không có (chỉ đọc, trả về nội tuyến) | tính nhất quán_check |
| `commit_chapter` | chapters/chXX.md + Progress | commit |
| `save_review` | đánh giá/chXX.json (toàn cầu là chXX-global.json) | đánh giá |
| `save_arc_summary` | summaries/arc-vNNaNN.json | arc_summary |
| `save_volume_summary` | summaries/vol-vNN.json | volume_summary |
| `save_foundation` | foundation/*.json | premise / outline / layered_outline / characters / world_rules / expand_arc / append_volume / update_compass / complete_book |

`commit_chapter` giả định phát hiện hoàn thành cung/tập/cuốn sách và trả về 19 trường dữ kiện (`arc_end` / `needs_expansion` / `book_complete`, v.v.; `rule_violations` được thêm vào khi bật kiểm tra quy tắc cơ học). `save_review` thực hiện nâng cấp phán quyết (kiểm soát quyền truy cập thẻ điểm, hợp đồng bị bỏ lỡ → viết lại). Những logic này trước đây nằm rải rác trong lớp chính sách giờ đây đã được củng cố bên trong công cụ.

`edit_chapter` là một trình bao bọc mỏng xung quanh `agentcore.EditTool` và việc kiểm tra quyền sở hữu đảm bảo rằng các chương đã hoàn thành phải nằm trong `PendingRewrites` mới được chỉnh sửa.

### 5.3 Phân tầng lỗi

| Loại Lỗi | Lớp xử lý | Hành động |
|---|---|---|
| Hết giờ mạng/Truyền phát EOF | Công cụ | Thử lại 3 lần |
| nhà cung cấp 429/503 | văn học | chuyển đổi dự phòng sang nhà cung cấp dự phòng |
| Xác thực/Mẫu không tồn tại | Công cụ | Tải lên thiết bị đầu cuối |
| Thiếu tạo phẩm tiên quyết | Công cụ | Ném xung đột, LLM điều chỉnh `novel_context` rồi thử lại |
| Thông số công cụ bất hợp pháp | Công cụ | Xác thực được đưa ra, LLM thay đổi tham số |
| MaxTurns cạn kiệt | lõi đặc vụ | chạy xong, Host gửi xong |
| Tin nhắn không tuân thủ LLM (dừng chỉ suy nghĩ, v.v.) | lõi tác nhân (`llm/litellm.go` `convertMessages`) | Vào ngăn xếp + Lọc ra khỏi ngăn xếp; Máy chủ không biết |
| Truyền phản hồi trống rỗng / suy nghĩ lâu | Litellm (`StreamIdleTimeout=5min`) | cơ quan giám sát kích hoạt thử lại |

### 5.4 Bình thường

Mỗi công cụ viết trước tiên sẽ kiểm tra điểm kiểm tra trước khi thực thi nó: nếu `Step+Digest` của điểm kiểm tra mới nhất của phạm vi hiện tại giống với điểm kiểm tra này thì sản phẩm hiện có sẽ được trả về trực tiếp. LLM có thể tự tin thử lại mà không có chương trùng lặp hoặc tiến độ sai lệch.

---

## 6. Lắp ráp đại lý

> Một Lời nhắc rất lớn + một Tác nhân về mặt lý thuyết có thể hoàn thành một cuốn sách, nhưng có ba điều sẽ cản trở sự ổn định: **Bùng nổ bối cảnh** (dù nén mạnh đến đâu trong 200 chương, nó sẽ suy giảm), **Sự can thiệp vào trách nhiệm** (lập kế hoạch chặt chẽ/trí tưởng tượng viết/đánh giá phê bình pha loãng lẫn nhau trong cùng một lời nhắc), **Mất tiền thưởng không đồng nhất của mô hình** (Sử dụng Opus để lập kế hoạch, Sonnet để viết và Pro để đánh giá. Lựa chọn mô hình độc lập là một không gian đáng kể để tối ưu hóa chi phí/chất lượng trong các tác phẩm dài hạn). Do đó, cấu trúc liên kết đa tác nhân là cần thiết.

### 6.1 Coordinator

Trình điều khiển vòng lặp chính duy nhất. Được lắp ráp trong `internal/agents/build.go`:

```go
agent := agentcore.NewAgent(
    agentcore.WithModel(coordinatorModel),
    agentcore.WithSystemPrompt(bundle.Prompts.Coordinator),
    agentcore.WithTools(subagentTool, contextTool),
    agentcore.WithMaxTurns(100_000),
    agentcore.WithToolsAreIdempotent(true),
    agentcore.WithMaxToolErrors(0),  // subagent Không có cầu chì
    agentcore.WithMaxRetries(subagentMaxRetries),
    agentcore.WithContextManager(...),
    agentcore.WithStopGuard(reminder.NewStopGuard(store, nil)),
    agentcore.WithToolGate(completePhaseGate(store)),  // phase=complete chặn cứng subagent phân phát
)
```

Trách nhiệm: Chọn trình lập kế hoạch khi bắt đầu → Chu trình hoàn thành kế hoạch → Nhận `[Host Ra lệnh]` và tạo ngay tool_call `subagent` tương ứng → Xử lý quyết định độc lập `[sự can thiệp của người dùng]` → Tóm tắt đầu ra sau `book_complete=true`.

Không làm: ghi tệp, đọc trực tiếp Tiến trình (sử dụng tiểu thuyết_context) và quyết định bước tiếp theo khi lệnh Máy chủ đến.

> **Tại sao không xóa Điều phối viên và để Chủ nhà điều chỉnh proxy trực tiếp? ** Trông có vẻ "sạch sẽ hơn", nhưng bốn thứ sẽ bị mất: (1) Việc ra quyết định "làm gì tiếp theo" được giữ lại trong lớp LLM, lớp này được hưởng lợi trực tiếp từ việc nâng cấp mô hình; (2) Phán quyết nhẹ nhàng của phán quyết xem xét (chấp nhận/đánh bóng/viết lại + phạm vi ảnh hưởng) được chuyển ra khỏi mã cờ vây; (3) Đánh giá tác động của người dùng Steer được chuyển sang mô hình - “Động lực của vai phụ cần rõ ràng hơn” chương nào cần viết lại, Điều phối viên có thể đánh giá, nhưng Host mã hóa cứng thì không; (4) Các nhánh bất thường (phản hồi phác thảo của người viết, phát hiện lỗ hổng trong thế giới quan của người biên tập) do chính mô hình xử lý, tránh phải viết máy trạng thái Go cho từng nhánh. **Xóa Điều phối viên tương đương với việc thay đổi đặt cược từ "mô hình trở nên mạnh hơn" thành "mã cờ vây của tôi trở nên mạnh hơn" - đây không phải là một cược tốt**.

### 6.2 Cấu trúc liên kết tác nhân phụ và tính không đồng nhất của mô hình

```
Coordinator (1 agent run, MaxTurns=100_000)
    ↓ subagent()
architect_short/long  ·  writer  ·  editor
    ↓ Cuộc gọi công cụ
Store (Phương tiện cộng tác, không có giao tiếp trực tiếp giữa các đại lý phụ)
```

Số lượt quay của đại lý phụ là độc lập (bản địa của đại lý) và không tính đến hạn ngạch 100_000 lượt của Điều phối viên. Các đại lý phụ giao tiếp với nhau thông qua các tạo phẩm có cấu trúc trong Cửa hàng. Điều phối viên chỉ truyền "mô tả nhiệm vụ" và không di chuyển nội dung.

`bootstrap.ModelSet` hỗ trợ các mô hình cấp vai trò: điều phối viên/kiến trúc sư/người viết/biên tập viên được cấu hình độc lập + chuyển đổi dự phòng của nhà cung cấp. Nhà văn chạy Sonnet thay vì Opus có thể tiết kiệm chi phí rất lớn cho một cuốn tiểu thuyết 200 chương.

### 6.3 Ba loại mô hình cộng tác

Không có giao tiếp trực tiếp giữa các tác nhân phụ, tất cả thông tin đều chảy qua các tạo phẩm có cấu trúc trong Cửa hàng. Ba loại chế độ bao gồm tất cả các quy trình công việc của hệ thống:

**Chế độ A · Chuyển giao nối tiếp (Đường trục)**: Điều phối viên → Lập kế hoạch kiến ​​trúc → Người viết Chương 1..N → Đánh giá cuối phần của người biên tập → Viết lại người viết. Ở chế độ phổ biến nhất, Điều phối viên kiểm tra trạng thái hiện tại thông qua `novel_context` để xác định ai sẽ chuyển tiếp.

**Chế độ B · Xem lại phản hồi (vòng kín)**: Người viết phát hiện ra sai lệch phác thảo trong bản nháp → Giá trị trả về `commit_chapter` mang trường `writer_feedback` → Điều phối viên xem phản hồi và xác định xem có nên nâng cấp lên kiến ​​trúc sư hay không và gọi để điều chỉnh phác thảo. Người viết không gọi trực tiếp cho Kiến trúc sư, phản hồi được gửi lại cho Điều phối viên thông qua các trường có cấu trúc.

**Chế độ C · Mở rộng bộ xương (lập kế hoạch cán)**: `commit_chapter` phát hiện vòng cung tiếp theo vẫn là bộ xương → quay lại `arc_end_reached + next_skeleton_arc` → Flow Router gửi hướng dẫn → Điều phối viên điều chỉnh architecture_long để mở rộng chương chi tiết của vòng cung tiếp theo → Người viết tiếp tục. Khả năng "lập kế hoạch luân phiên" dạng dài là việc triển khai vòng kín này.

### 6.4 Ràng buộc mã cho các quy trình tác nhân phụ (không cần dựa vào lời nhắc)

> Quá trình viết ban đầu dựa vào ràng buộc "tiến triển nghiêm ngặt theo thứ tự sau" của `writer.md`. LLM thường bị vi phạm - bỏ qua kế hoạch và đi thẳng vào bản nháp, tiếp tục nói một đoạn khác sau khi cam kết sẽ tiêu tốn mã thông báo và chỉ viết văn bản chính trong cuộc trò chuyện mà không đặt hàng. **Quy trình ràng buộc từ nhắc nhở không ổn định**. Sức mạnh hoàn toàn phụ thuộc vào sự “ngoan ngoãn” hiện tại của người mẫu. Việc nâng cấp mô hình có thể khiến nó “không tuân theo một cách sáng tạo”.

Bốn cấp độ ràng buộc mã (có hiệu lực cùng lúc):

| Lớp | Điểm rơi | Chức năng |
|---|---|---|
| `StopAfterTools` / `StopAfterToolResult` | Cấu hình con `agents/build.go` | Công cụ then chốt là end_turn thành công để thoát khỏi quá trình chạy tác nhân phụ. Trình ghi `commit_chapter` dừng khi nhấn (`StopAfterTools`); `save_arc_summary`/`save_volume_summary` của người biên tập, phần kết thúc tập/cung của Kiến trúc sư sử dụng `StopAfterToolResult`. `save_review` của biên tập viên không dừng cứng - nếu không sẽ bỏ qua StopGuard và chạy tóm tắt vòng cung, đóng chuyển giao `NewEditorStopGuard` |
| `CheckpointDeltaGuard` | `host/reminder/subagent_guards.go` | Với điểm kiểm tra cơ sở làm ranh giới, điểm kiểm tra mới tương ứng với bước phải được nhìn thấy trước khi kết thúc vòng này, nếu không `end_turn` sẽ bị từ chối; chấm dứt nâng cấp ba lần liên tiếp (vòng lặp vô hạn mô hình yếu) |
| Công cụ nội tuyến `next_step` | Mỗi trường giá trị trả về của công cụ | Mỗi thực tế đều có "gợi ý bước tiếp theo" riêng. Ví dụ: `plan_chapter` trả về `next_step: "Gọi ngay draft_chapter..."`. LLM biết bước tiếp theo khi nhìn thấy sự thật mà không cần phải quay lại lời nhắc hệ thống để tìm |
| Phân bổ/kiểm tra trước trong công cụ | `edit_chapter` `commit_chapter`, v.v. | Chặn vật lý lớp dữ liệu: `edit_chapter` từ chối thay đổi các chương đã hoàn thành không có trong `PendingRewrites`; `commit_chapter` từ chối các bài gửi trống trong đó bản nháp == bản nháp cuối cùng; `ConcurrencySafe=false` ngăn chặn các cuộc đua đồng thời |

Trong cấu trúc mới, writer.md chỉ chịu trách nhiệm: viết hướng dẫn chất lượng, mô hình nhận thức điểm dừng và tiếp tục cũng như giải thích hợp đồng chương. **Không cần lập kế hoạch quy trình nữa** - lời nhắc sẽ không lưu lại tình huống khi LLM bỏ qua các bước, mã sẽ lưu lại. Bốn lớp ràng buộc tương tự dành cho kiến ​​trúc sư/người soạn thảo đều có trong các công cụ/Guard tương ứng của chúng.

> Về quy tắc bọc thép thứ nhất: `next_step` là một tuyên bố thực tế nội tuyến trong công cụ ("Tôi vừa lưu kế hoạch"), không phải là một quá trình điều phối việc chèn cuộc gọi chéo máy chủ. Lập kế hoạch giữa các tác nhân phụ ở lớp Điều phối viên vẫn tuân thủ nghiêm ngặt Flow Router → Steer.

### phụ thuộc vào lõi tác nhân 6.5

`../agentcore` là thư viện Tác nhân chung của dự án này (được liên kết với go.work). Tất cả các nguyên hàm được sử dụng trong kiến ​​trúc mới đều đã tồn tại: `Prompt`/`Inject`/`Steer`/`Subscribe`/`WithMaxTurns`/`WithStopGuard`/`WithToolGate`/`WithMiddlewares`/`SubAgentConfig`/`WithContextManager`.

**Sửa đổi ranh giới**:

- Nhập lõi tác nhân: chiến lược Trình quản lý bối cảnh mới, điều chỉnh nhà cung cấp mới, loại sự kiện mới, chế độ chèn thông báo chung
- Không nhập lõi tác nhân: các mô hình kinh doanh như Tiến trình/Điểm kiểm tra/Phạm vi, các công cụ kinh doanh như tiểu thuyết_context/commit_chapter và các quy tắc kinh doanh như kiểm soát truy cập/phát hiện kết thúc vòng cung

Tiêu chí phán đoán: Giả sử rằng tác nhân mã hóa/tác nhân dịch vụ khách hàng sẽ được giới thiệu trong tương lai, các khả năng mới sẽ chỉ được phép đưa vào nếu chúng vẫn có ý nghĩa trong kịch bản đó. **Cấm viết các bản vá bí mật ở lớp ứng dụng** (agent, Wrapper, Monkey Patch) - Nếu không đủ khả năng, hãy trực tiếp đến Agentcore để thay đổi.

**Các khả năng không được cố ý sử dụng** (để tránh lạm dụng):

- `Agent.TaskRuntime() / Tasks() / StopTask()` — tác nhân phụ nền có tính năng chống cháy và quên được tích hợp sẵn của Agentcore. Trong kiến ​​trúc mới, tất cả các cuộc gọi của tổng đài viên phụ đều được đồng bộ hóa ở nền trước, **không được sử dụng**
- `Agent.Steer(msg)` - kênh lệnh xử lý của `flow.Dispatcher`, dùng để đưa `[Host Ra lệnh]` vào quá trình chạy Cođiều phối viên đang chạy; nó phải được kích hoạt ở ranh giới công cụ đồng bộ hóa để đảm bảo rằng kết quả công cụ được phân phối trước lệnh gọi mô hình tiếp theo
- `Agent.FollowUp(msg)` - Kênh thông báo theo dõi nhàn rỗi, không sử dụng cho Flow Router; nó chỉ được làm trống khi tác nhân chuẩn bị dừng lại một cách tự nhiên. Việc sử dụng nó để đưa ra hướng dẫn quy trình chính sẽ khiến hướng dẫn đến muộn.
- `Agent.Inject(msg)` / `InjectContext` - lối vào can thiệp của người dùng/bên ngoài: ghi vào hàng đợi điều khiển trong khi chạy và tự động tiếp tục chạy khi không hoạt động và có thể tiếp tục; `Steer(text)` của Host sử dụng nó, Resume sử dụng `Prompt` để bắt đầu lần chạy mới
- `WithPermission*` — cơ chế phê duyệt quyền (phê duyệt thủ công các hoạt động nguy hiểm), các ứng dụng mới không có hoạt động nguy hiểm, **không được sử dụng**

**Đã bật móc chính sách**: `WithToolGate` - Mục đích duy nhất là chặn cứng việc gửi `subagent` (`agents/build.go` `completePhaseGate`) khi `phase=complete`. Sau khi hoàn thành, nếu người dùng yêu cầu tiếp tục/viết lại, Điều phối viên LLM vẫn có thể gửi tác nhân phụ của riêng mình và việc viết vượt quá giới hạn của Người viết sẽ bị `commit_chapter` từ chối, `CheckpointDeltaGuard` sẽ không cho phép điều đó và `end_turn` → một vòng lặp vô hạn. Khi Flow Router trả về 0 khi hoàn tất, nó chỉ chặn việc gửi tự động của Máy chủ chứ không thể chặn việc gửi LLM tự động. Do đó, Gate bổ sung thêm biện pháp bảo vệ trạng thái cuối cùng tại điểm nghẹt thở. Đây là một quy trình có mục đích hẹp và không phải là quy trình phê duyệt như `WithPermission*`. Cả hai không nên nhầm lẫn.

---

## 7. Lớp máy chủ

### 7.1 Cấu trúc

```go
type Host struct {
    cfg               bootstrap.Config
    bundle            assets.Bundle
    store             *store.Store
    models            *bootstrap.ModelSet
    coordinator       *agentcore.Agent
    coordinatorCtxMgr *corecontext.ContextEngine  // Liên kết cửa sổ ngữ cảnh khi cắt mô hình
    askUser           *tools.AskUserTool
    writerRestore     *ctxpack.WriterRestorePack

    observer     *observer
    router       *flow.Dispatcher  // Đồng bộ hóa ranh giới công cụ + Route + Steer
    usage        *UsageTracker
    usageCancel  context.CancelFunc
    budget       *BudgetSentinel   // Host Thành phần chính sách: Thực thi báo cáo ngân sách người dùng (tương đương với Abort), ranh giới đồng bộ hóa có trước Dispatcher
    notifier     *notify.Notifier  // Lớp quan sát:run_end/repeat/budget Bản sao ngoài màn hình của ba loại cảnh báo, không bao giờ can thiệp vào luồng điều khiển

    events, streamCh, done chan ...

    mu        sync.Mutex
    lifecycle lifecycle  // idle / running / paused / completed
    closeOnce sync.Once
}
```

### 7.2 API công khai

**Vòng đời** (Mục nhập Chạy của Điều phối viên): `Start` / `StartPrepared` / `Resume` / `Continue` / `Steer` / `Abort` / `Close`

**Kênh quan sát**: `Events`/`Stream`/`Done` (xóa trọng điểm trong luồngCh)

**Tổng hợp giao diện người dùng**: `Snapshot()` - TUI lấy tất cả dữ liệu hiển thị cùng một lúc

**Cấu hình/Tiện ích mở rộng**: Quản lý mô hình (`SwitchModel`), nhập ngược tiểu thuyết bên ngoài (`ImportFrom`), đối thoại đồng sáng tạo (`CoCreateStream`), phát lại sự kiện (`ReplayQueue`), chân dung mô phỏng (`Simulate`/`ImportSimulationProfile`), xuất (`Export`)

Không có phương pháp lập lịch dịch vụ như `decideNext` `retryActiveTask`. Bộ định tuyến luồng là sự kết hợp mỏng giữa các chức năng thuần túy + ​​Điều phối chỉ đạo và không giữ trạng thái ngầm định chẳng hạn như "tác vụ đang được thử lại".

### 7.3 dạng `waitDone`

```go
func (h *Host) waitDone() {
    h.coordinator.WaitForIdle()
    h.observer.finalize()

    if Phase == Complete { lifecycle=completed; tóc"Quá trình tạo đã hoàn tất"sự kiện }
    else if running        { lifecycle=idle;     tóc"Coordinator dừng lại (Hoàn thành N chương)"sự kiện }

    select { case h.done <- struct{}{}: default: }
}
```

Ba điều: chờ ở trạng thái rảnh → vòng đời chuyển đổi → gửi sự kiện cuối cùng + gửi tín hiệu hoàn tất. ** Vô hiệu hóa `Inject` / `FollowUp` / `Prompt` xuất hiện trong nội dung hàm **. Sau khi LLM hoàn thành Chạy, toàn bộ Máy chủ sẽ chuyển sang trạng thái cuối cùng.

Chỉ có hai cách để bắt đầu lại: người dùng chủ động sử dụng `Continue`/`Start` hoặc khởi động lại quy trình và sử dụng `Resume`.

> Bài học lịch sử: Một bản vá để `idleResumeCount` tự động khởi động lại Run đã được thêm vào chức năng này. Trong thời gian dài mimo duy nhất thực sự được kích hoạt, nó vô dụng 100%. Thay vào đó, nó che đậy nguyên nhân thực sự của việc "ngăn chặn các tin nhắn đi vào lịch sử chỉ có suy nghĩ" trong lớp Agentcore. **Việc "khởi động lại phòng thủ" của lớp Máy chủ luôn là một sửa chữa sai vị trí**. Xem `feedback_no_host_resilience.md` và §10 Điều 5 để biết chi tiết.

---

## 8. Khởi động và phục hồi

### 8.1 Mới

```
User: "yêu cầu một câu"
  → Host.Start
    → store.Progress.Init / store.Checkpoints.Reset
    → coordinator.Prompt(userPrompt) + flow.Dispatcher.Enable + Dispatch
    → Coordinator long loop: lập kế hoạch → Viết 1..N → Ôn tập → done
```

### 8.2 Recovery (khởi động lại sau sự cố)

```
quá trình bắt đầu
  → đọc Progress + gần đây Checkpoint + PendingCommit + PendingSteer
  → buildResumePrompt → thông báo ngắn (không step hướng dẫn cấp độ)
  → coordinator.Prompt(resumePrompt) + Dispatcher.Enable + Dispatch
  → Coordinator theo Host Hướng dẫn tiếp tục
```

Tiếp tục sử dụng `Prompt` để bắt đầu lần chạy mới (đặt lại số lượt, làm sạch ngữ cảnh), không phải `FollowUp`. Bước rõ ràng đầu tiên sau khi khôi phục được Flow Router `Dispatch` thực hiện ngay lập tức, các bước tiếp theo được bắt nguồn từ ranh giới đồng bộ hóa được công cụ tác nhân phụ trả về thành công.

### 8.3 Sự can thiệp của người dùng

| Nhập cảnh | Tiền tố | Ngữ nghĩa | Thực hiện |
|---|---|---|---|
| `Steer(text)` | `[sự can thiệp của người dùng]` | Sửa đổi/truy vấn cần có quyết định của Điều phối viên | Đi đến `Inject` trong quá trình hoạt động; ghi PendingSteer vào `meta/run.json` trong khi tắt máy |
| `Continue(text)` | `[sự can thiệp của người dùng]` | Viết tiếp, thức dậy sau khi tắt máy | Đi tới `FollowUp` trong khi chạy; vào `Inject` khi dừng, tự động tiếp tục chạy |

Hai lối vào được thống nhất thông qua trình trợ giúp `interventionMsg` cộng với tiền tố `[sự can thiệp của người dùng]` - đó là điểm neo để phân loại can thiệp `coordinator.md`; trước đây khi Continue đăng văn bản khỏa thân sẽ bỏ qua việc phân loại và bị gán nhầm cho người viết để thay đổi chương viết (đã sửa đổi).

Ngữ nghĩa `Inject`: chèn hàng đợi chạy hiện tại trong quá trình hoạt động; tự động tiếp tục chạy và tiêm khi không hoạt động; xếp hàng và chờ phục hồi khi tạm dừng.

**Lớp kiên trì can thiệp lâu dài**: Trong hạng mục can thiệp, "yêu cầu dài hạn chỉ ảnh hưởng đến việc viết tiếp theo" (loại kiểu/xu hướng) được Điều phối viên chuyển từ `save_directive` sang `meta/user_directives.json` (tối đa 20 mục, thêm bớt trùng lặp/xóa theo số sê-ri), `novel_context` được đưa vào `working_memory.user_directives` - tất cả các tác nhân phụ tự động xem từng chương, có hiệu lực khi nén và khởi động lại, đồng thời không dựa vào bộ nhớ đối thoại và phân phối lệnh của Điều phối viên. Ba loại giải pháp can thiệp còn lại đã có sẵn trong kho (độ dài→la bàn/phác thảo, thiết lập→nền tảng, thay đổi các chương cũ→Đang chờ xử lý lại). Lấy phong bì nhưng không lấy lời nhắc hệ thống: Bảo vệ bộ nhớ đệm tiền tố hệ thống nhiều chương của người viết.

Mỗi hướng dẫn được đính kèm với **ảnh chụp nhanh tiến trình** khi nó được ban hành (at_chapter / at_total_chapters): hướng dẫn có hiệu lực ngược từ at_chapter (người soạn thảo không truy ngược lại chương cũ); trong trường hợp lệnh tương đối ("Thêm 10 chương") bị lưu nhầm thành yêu cầu dài hạn, người đọc có thể đánh giá dựa trên ảnh chụp nhanh rằng đã đáp ứng và sẽ không được thực hiện nhiều lần. Cách chính xác cho các hướng dẫn dựa trên hành động vẫn là dịch thời gian ghi của lộ trình tương ứng (trạng thái tuyệt đối của kiến ​​trúc sư/người biên tập → phác thảo/la bàn/Đang chờ xử lý) và ảnh chụp nhanh là bảo hiểm trong trường hợp phân loại sai.

---

## 9. Cấu trúc thư mục

```
internal/
  domain/         Dữ liệu thuần túy:Phase / FlowState / Progress / Checkpoint / Scope / Story / Plan /
                  Review / StateChange / Phase-Flow Quy tắc di chuyển
  store/          Tính kiên trì của hệ thống tập tin (tmp+rename + Bộ ba mảnh):progress / checkpoints / outline /
                  drafts / summaries / characters / world / signals / run_meta / runtime / session
  tools/          11 cá nhân Agent Bộ dụng cụ ba món dành cho tất cả các lớp viết + digest bình thường + ConcurrencySafe=false
                  + premise_structure (save_foundation Để sử dụng nội bộ) + ask_user
  agents/         build.go cuộc họp Coordinator + đại lý Sanzi;ctxpack/ Writer Chiến lược nén ngữ cảnh
  host/           host.go + resume.go + observer.go + events.go + usage.go + usage_replay.go
                  + stream_extract.go + cocreate.go
    flow/         router.go (hàm thuần túy 11 chi nhánh) + state.go + dispatcher.go + router_test.go
    reminder/     stop_guard.go (Coordinator) + subagent_guards.go (CheckpointDeltaGuard ×3)
    imp/          Nhập ngược tiểu thuyết bên ngoài:split → foundation → Phân tích từng chương
    exp/          Xuất chương đã hoàn thành: hợp nhất các chương → TXT / EPUB 3, trình điều khiển hậu tố đường dẫn; hoàn toàn chỉ đọc, không phụ thuộc vào LLM
  entry/          tui (Bubble Tea) / headless / startup
  bootstrap/      config + ModelSet + provider failover + setup thuật sĩ
  models/         OpenRouter vv. đăng ký mô hình công cộng + làm mới giá (24h bộ đệm đĩa)
  errs/           phân tầng sai
  diag/           đăng ký host Mô-đun chẩn đoán chỉ đọc cho luồng sự kiện
  utils/          Di sản của kiến ​​trúc cũ (một số ít công cụ phân tích cú pháp, không nên dựa vào mã mới)

assets/
  prompts/        coordinator (~55 ĐƯỢC RỒI) / architect-short|long / writer / editor / import-* / simulation-*
  references/     kỹ năng viết + Mẫu thể loại + Lập kế hoạch dài hạn, v.v.
  styles/         mặc định/tưởng tượng/Lãng mạn/Hồi hộp

../agentcore     Phổ quát Agent khung(go.work Danh mục Brother, có thể thêm khả năng chung, không thêm doanh nghiệp)
../litellm       LLM cửa ngõ
```

### 9.1 Các mốc tiến hóa

| Thời gian | Tái cấu trúc | Hiệu ứng ròng |
|---|---|---|
| 2026-04-10 | `internal/orchestrator/` (6342 dòng) → `host/` + `agents/` | Lõi thời gian chạy -74% |
| 2026-04-20 | Điều phối viên lai: Tạo `host/flow/` mới, thu gọn `reminder/`, `coordinator.md` 88 dòng → 45 dòng | Tỷ lệ lỗi định tuyến tiệm cận 0 |
| 2026-05-02 | lõi tác nhân `WithMaxToolErrors(0)` + `isReasoningOnlyStopAssistant`; `StreamIdleTimeout=5min`; xóa bản vá tiếp tục `idleResumeCount` | mimo / suy nghĩ chậm về phát trực tuyến |
| 2026-06-05 | Lập kế hoạch cuộn vòng khép kín (`expand_arc`/`append_volume`) + Tiếp tục phân lớp ngược `/import` + can thiệp vào không gian người dùng | Hơn 200 chương đầu tiên xem qua |

Đo lường thực tế: hy3-preview miễn phí 12 chương/73 phút, mimo-v2.5-pro 10 chương/84.000 từ (trung bình chương 8400), cả hai đều hoàn thành trong một lần chạy; bản dài gpt-5.4 "Mortal Bones" 235 chương / 1,27 triệu từ / chương trung bình 5407, chạy theo kế hoạch cuốn vòng khép kín.

---

## 10. Nói rõ những gì không nên làm

Một sự vi phạm thể hiện sự sai lệch về mặt kiến ​​trúc.

1. **Không đưa ra khái niệm Task/Job/WorkItem**. "Nhiệm vụ hiện tại" được giao diện người dùng hiển thị là một phép chiếu luồng sự kiện chứ không phải thực tế.
2. **Không giới thiệu Người điều phối / Người lập lịch trình / Người đánh giá sẵn sàng **. Quyền ra quyết định nằm ở cấp độ Điều phối viên LLM và công cụ.
3. **Không triển khai cơ chế "sơ yếu lý lịch nhàn rỗi" giống như `idle_dispatch`**. Điều phối viên Chạy kết thúc = Máy chủ gửi xong.
4. **Không bỏ qua Điều phối viên và gọi trực tiếp cho Đại lý phụ** trên Máy chủ. Bộ định tuyến luồng phát hành `[Host Ra lệnh]` đến `coordinator.Steer`, cho phép Điều phối viên tạo tool_call. Tiếp tục bắt đầu một lần chạy mới bằng `Prompt`.
5. **Không thêm bản vá tiếp tục tự động khi LLM tắt bất thường trên Máy chủ**. Kết thúc chạy = Máy chủ chuyển sang trạng thái cuối cùng. `idleResumeCount` cũ đã bị xóa (xem §7.3, `feedback_no_host_resilience.md` để biết chi tiết).
6. **Không suy luận việc hoàn thành nhiệm vụ dựa trên "kết thúc thực thi công cụ"**. Bằng chứng duy nhất của việc hoàn thành là ghi điểm kiểm tra.
7. **Không thực hiện các mô hình bốn lớp như WorkflowInstance / TaskInstance / Command + Apply**. Chỉ có ba loại lớp thực tế: Tiến trình + Điểm kiểm tra + Hiện vật.
8. **Không hỗ trợ các tác vụ song song**. Điều phối viên hoạt động duy nhất Chạy, thăng tiến nối tiếp một cuốn sách. Vui lòng sử dụng nhiều quy trình cho nhiều tiểu thuyết.
9. **Không thực hiện lệnh gọi LLM** ở lớp công cụ (ngoại trừ chính công cụ Tác nhân). IO thuần túy + ​​xác minh + bình thường.
10. **Không để giao diện người dùng đọc trực tiếp Cửa hàng**. Chỉ có thể đăng ký sự kiện hoặc đọc Host `Snapshot()`.
11. **Thực hiện IPC mà không cần tập tin tín hiệu**. Máy chủ đọc trực tiếp Tiến trình + Điểm kiểm tra + phác thảo phân cấp, hướng dẫn xuất phát `flow.Route` từ thực tế là định tuyến dọc hợp lý.
12. **Không ghi máy trạng thái Flow ở phía Máy chủ**. Thẻ luồng chỉ được cập nhật bằng công cụ, Router chỉ đọc và không ghi.
13. **Không mã hóa "ảo ảnh LLM"**. Tối ưu hóa lời nhắc, cải thiện cấu trúc giá trị trả về của công cụ và để `novel_context` trình bày sự thật rõ ràng hơn - thay vì buộc máy chủ phải thay đổi quy trình.
14. **Không để lớp chẩn đoán/quan sát can thiệp vào luồng điều khiển**. Tìm kiếm chẩn đoán chỉ đọc, chỉ sản xuất và xuất khẩu không nhạy cảm; Quá trình sửa chữa/tiếp tục/sửa đổi tự động sẽ không được thực hiện (xem §2.3 Kỷ luật quan sát viên).
15. **Ngân sách và cảnh báo không nhập vào lớp Tuyến đường/Công cụ và cảnh báo không nhập luồng điều khiển**. `BudgetSentinel` là thành phần Chính sách máy chủ (thực thi Hủy bỏ do người dùng ký trước, không đánh giá hành vi của mô hình); `notify` là quan sát thuần túy (không thử lại, không lên lịch lại, không có thời gian ngừng hoạt động). `flow.Route` vẫn là một chức năng thuần túy và không có nhận thức về cả hai chức năng này.

---

## 11. Chiến lược xác minh

### 11.1 Kịch bản ổn định

- **Chạy đường dài**: Chương 80~200 có thể chạy một lần, Giai đoạn=hoàn thành. Cho phép chuyển đổi dự phòng của nhà cung cấp và thử lại tạm thời các công cụ; cấm Máy chủ tiếp tục chạy hoặc Điều phối viên chạy nhiều lần.
- **B Phục hồi sự cố**: Chương N sau bản nháp/trước quá trình hủy cam kết → Tiếp tục → Tiếp tục từ tính nhất quán_kiểm tra mà không viết lại bản nháp bị bỏ. `checkpoints.jsonl` không có bước lặp lại.
- **jitter của nhà cung cấp C**: Mô phỏng không liên tục 503 → chuyển đổi dự phòng litellm; Vòng lặp chính LLM không nhận biết được.
- **D Can thiệp của người dùng**: Khi chạy, thao tác Chỉ đạo → Điều phối viên được xử lý ở lượt tiếp theo; sau khi tắt máy, lời nhắc Chỉ đạo → Tiếp tục tiếp theo sẽ được bao gồm.

### 11.2 Tuân thủ (có thể viết là linter/test)

- `internal/host/` không cho phép lập lịch các gói như `import "internal/scheduler"`
- Số lượng API vòng đời của `host.go` ổn định; các phương thức công khai mới chỉ có thể là các lớp "mục mở rộng" (đồng sáng tạo/nhập/quản lý mô hình)
- `coordinator.Inject`/`FollowUp`/`Prompt` không được phép trong thân hàm `waitDone`
- Mã liên quan đến `recovery` chỉ có thể xuất hiện trong `host/resume.go`
- `flow.Route` phải là hàm thuần: cấm đọc Store/bất kỳ IO nào

### 11.3 Lặp lại chất lượng

Thay đổi `writer.md` sẽ ngay lập tức tạo ra những thay đổi về kiểu dáng; thứ nguyên đánh giá trình chỉnh sửa mới sẽ tương thích ngược (save_review nhận JSON có cấu trúc). Việc thêm một md tham chiếu mới yêu cầu ba kết nối (trường `tools.References` + `loadReferences` của `assets/load.go` + nội dung `writerReferences`/`architectReferences` của `novel_context.go`). Nó được tải tự động mà không được đưa vào thư mục - `References` là một ánh xạ trường rõ ràng, tạo điều kiện thuận lợi cho việc điều chỉnh theo vai trò/chương.

**Thống kê về phong cách cấp độ toàn bộ cuốn sách (`internal/stylestat`)**: Cửa sổ xem lại trong phần này kiểm tra "mẫu câu tic các chương hàng chục lần, sự đồng hình hình thái ở cuối chương và sự lặp lại từng từ trong nhiều chương", vốn là chứng mù tự nhiên được củng cố ở toàn bộ cấp độ cuốn sách - mọi thứ trong một chương đều bình thường. Đường dẫn chương `novel_context` chạy số liệu thống kê xác định trên tất cả các chương đã hoàn thành (danh mục mẫu câu/cụm từ tần suất cao gần cửa sổ/các câu lặp lại nhiều chương/dạng cuối chương/định dạng tiêu đề được trộn lẫn với nhau) và được đưa vào `episodic_memory.style_stats`: người biên tập quyết định bằng số theo khía cạnh thẩm mỹ và người viết tránh điều đó theo đó. **Thống kê thuộc về mã và phán quyết thuộc về LLM** - ngưỡng không được ghi trong mã và con số có đúng hay không sẽ được mô hình đánh giá theo chủ đề. Dòng dưới cùng của sản phẩm song song `rules.Lint` (phần còn lại đánh dấu/đoạn không phải tiếng Trung) luôn được thực thi trong commit_chapter và chỉ trả về dữ kiện.

---

## 12. Tóm tắt

> **Hãy để LLM viết xong một cuốn tiểu thuyết trong một lần. Máy chủ chỉ thực hiện khởi động/khôi phục/định tuyến/quan sát. Bản ghi thực tế được công cụ đặt một cách nguyên tử và quyền ra quyết định được giao cho mô hình càng nhiều càng tốt. **

Không có công cụ xử lý công việc, không có hàng đợi nhiệm vụ, không có người điều phối, không có bộ lập lịch. Một số chỉ là:

- Điều phối viên lượt 100_000
- Ba loại tác nhân phụ chức năng (ngữ cảnh và mô hình độc lập)
- 11 công cụ nguyên tử
- một tập tin điểm kiểm tra jsonl
- ~860 dòng Host shell
- ~150 dòng chức năng thuần túy của Flow Router (11 nhánh + thử nghiệm đơn)

Mỗi dòng mã doanh nghiệp Máy chủ đều là một biện pháp phòng ngừa rủi ro cho việc nâng cấp mô hình. **Máy chủ nhỏ nhất, Lời nhắc lớn nhất (lớp chất lượng) và các công cụ mạnh mẽ nhất** làm cho kiến ​​trúc tự động trở nên tốt hơn hàng năm - Điều phối viên đưa ra quyết định chính xác hơn, Người viết viết tốt hơn, Người chỉnh sửa đánh giá chính xác hơn và Kiến trúc sư lập kế hoạch chính xác hơn, tất cả đều là lợi ích của việc thay đổi trực tiếp kiến ​​trúc mô hình.

Ngược lại, nếu các quy tắc như "đánh giá cuối cùng được yêu cầu viết lại Chương 3 và 5" hoặc "dừng nếu không có tiến bộ ba lần liên tiếp" được mã hóa cứng trong Máy chủ, thì việc nâng cấp mô hình sẽ khiến nó trở thành kết quả âm: các phán đoán mà LLM nên thực hiện trở nên dư thừa và logic bảo vệ trở thành dương tính giả. **Tệ nhất là không ai dám xóa - xóa tương đương với việc "tin người mẫu", gánh nặng tâm lý còn khó dọn hơn mã**. Càng để lại nhiều mã này thì chi phí tái cấu trúc trong tương lai càng cao.

**Khả năng mở rộng đến từ các điểm mở rộng phù hợp**: thay đổi kiểu → lời nhắc thay đổi; chiều đánh giá mới → lời nhắc thay đổi; chủ đề mới → thêm tài liệu tham khảo; loại tác nhân phụ mới → thêm một dòng SubAgentConfig; nhiều tiểu thuyết song song → nhiều quy trình.

Kỷ luật duy nhất: **Khi ai đó muốn "làm cho Người dẫn chương trình thông minh hơn", trước tiên hãy hỏi "Tại sao không làm cho LLM thông minh hơn"**. Nếu bạn không thể trả lời câu hỏi này tại sao "Máy chủ là cần thiết", đừng thêm mã vào Máy chủ.
