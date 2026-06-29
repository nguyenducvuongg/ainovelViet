# Đề xuất tái cấu trúc: Điều phối viên kết hợp — Định tuyến máy chủ × Phán quyết LLM

> Trạng thái: **Đã thông qua và triển khai** (2026-04-20)
> Thời gian nghiên cứu: 20/04/2026
> Tương ứng với các tài liệu hiện tại: `docs/architecture.md` §2 / §3 / §7 / ​​​​§8 / §13 đã được cập nhật đồng thời
>
> **Tài liệu này là bản dự thảo thứ hai. **Các vấn đề trong bản thảo đầu tiên của giải pháp căn cơ (xóa hoàn toàn Điều phối viên) được trình bày chi tiết tại Phụ lục A. Hãy giữ lại phần này để tránh lặp lại các đường vòng.
>
> Kết quả thực hiện:
> - `internal/host/flow/` mới được tạo (router.go/state.go/dispatcher.go/router_test.go, tất cả 15 bài kiểm tra đơn nhánh đều đã vượt qua)
> - `internal/host/reminder/` xóa `flow.go`/`queue_guard.go`/`book_complete.go`; giữ lại StopGuard và subagent Guard
> - `assets/prompts/coordinator.md` giảm từ 88 dòng xuống còn ~45 dòng (trách nhiệm được thu hẹp để thực thi hướng dẫn Máy chủ + phán quyết + lựa chọn khởi động)
> - `internal/host/resume.go` được đơn giản hóa rất nhiều và chỉ tạo nhãn và dấu nhắc ngắn. Bước tiếp theo cụ thể sẽ được Router gửi đi sau TurnEnd đầu tiên.
> - `internal/store/` thêm các phương thức trợ giúp `HasArcReview`/`HasArcSummary`/`HasVolumeSummary`/`CheckConsistency`
> - Lỗi trạng thái tác nhân `observer.go` không còn ngừng hoạt động cũng đã được sửa.

---

## 1. Bối cảnh

### 1.1 Định vị dự án

```
agentcore       — Phổ quát agent khung
litellm         — Phổ quát LLM cửa ngõ
ainovel-cli     — Viết tiểu thuyết theo chiều dọc agent(dự án này)
```

Không gian ra quyết định của các tác nhân dọc **đóng**: lưu đồ cố định, các nhánh bị giới hạn và dựa trên thực tế. Triết lý thiết kế của các tổng đại lý ("đặt cược vào khả năng của mô hình") bị nghi ngờ là quá thuần túy khi áp dụng cho các kịch bản theo chiều dọc.

### 1.2 Mục tiêu của người dùng (theo mức độ ưu tiên)

1. **Tính ổn định** — tiếp tục viết mà không bị gián đoạn do lỗi định tuyến
2. **Lợi ích nâng cấp Eat LLM** — kiến ​​trúc không cạnh tranh với khả năng của mô hình
3. **Tận dụng tối đa khả năng của nhiều tác nhân** — Phân chia chức năng rõ ràng

Đề xuất này tạo ra **cải thiện Pareto** giữa ba mục tiêu (không hy sinh bất kỳ mục tiêu nào để đổi lấy mục tiêu khác).

---

## 2. Khảo sát hiện trạng

### 2.1 Phân loại điểm quyết định của điều phối viên

Trích xuất từng điểm quyết định `coordinator.md`:

| # | Điểm quyết định | Thiên nhiên | Tần số |
|---|---|---|---|
| 1 | Chọn Architect_long/ngắn khi khởi động | Phán quyết (hiểu ngữ nghĩa) | Một cuốn sách 1 lần |
| 2 | Mở rộng đầu vào (tự động thêm <20 từ) | Phán quyết (sáng tạo) | 0-1 lần cho một cuốn sách |
| 3 | Vòng hoàn thành kế hoạch | Định tuyến (dựa trên thực tế) | 1-3 lần |
| 4 | Bước tiếp theo sau khi hoàn thành từng chương | **Định tuyến** | **1-2 lần mỗi chương** |
| 5 | Thực hiện từng bước đánh giá cuối vòng cung | Định tuyến | 3-5 lần mỗi cung |
| 6 | xem lại bản án ngã ba | định tuyến (được mã hóa, xem §2.3) | 1 mỗi cung |
| 7 | Xử lý can thiệp của người dùng | Phân xử (yêu cầu LLM) | Bất kỳ |
| 8 | Phân phối lại lỗi đại lý phụ | Định tuyến | Thỉnh thoảng |
| 9 | Tóm tắt đầu ra hoàn chỉnh của cuốn sách | Định tuyến | 1 lần |

**Kết luận**: 6 trong số 9 điểm quyết định là định tuyến thuần túy (tra cứu bảng) và 3 điểm là các phán quyết thực sự yêu cầu LLM. **Việc định tuyến diễn ra thường xuyên hơn nhiều so với việc phân xử** (1-2 lần mỗi chương so với nhiều lần mỗi cuốn sách).

### 2.2 Kênh nhắc nhở đã là sản phẩm bán thành phẩm của quá trình mã hóa quy trình

Trình tạo trong `internal/host/reminder/` tạo ra các hướng dẫn dành riêng cho hành động dựa trên dữ kiện trong mỗi vòng:

- `flow.go` → `"hiện hành flow=writing，next_chapter=37. Vui lòng gọi trực tiếp subagent(writer, \"viết cái đầu tiên 37 chương\")..."`
- `queue_guard.go` → `"hiện hành flow=rewriting, hàng đợi chờ:[3,5]. Hãy gọi ngay writer Viết lại từng chương..."`
- `book_complete.go` → `"Cuốn sách đã hoàn tất. Vui lòng xuất bản tóm tắt của cuốn sách..."`

**Công văn kép tồn tại trong kiến ​​trúc hiện tại**:
```
Lớp quy tắc:coordinator.md sự định nghĩa"nếu như A Nhưng B"
  ↓
Reminder Lớp: Mỗi vòng cụ thể hóa các quy tắc dựa trên thực tế → phát ra"hãy làm điều đó ngay bây giờ B"
  ↓
LLM Lớp: đọc reminder phát ra tool_call(Về cơ bản là kể lại reminder）
  ↓
SubAgent thực hiện
```

**LLM thực chất chỉ là "thực thi" các hướng dẫn được đưa ra bởi Reminder**. Liên kết trung gian này không chỉ tiêu thụ mã thông báo mà còn gây ra sự không chắc chắn (LLM có thể không tuân thủ đầy đủ các lời nhắc, chẳng hạn như lỗi định tuyến giữa chừng được quan sát thấy).

### 2.3 Lớp công cụ bị đánh giá nhiều

- `save_review.evaluateScorecardGate()`: Kiểm soát truy cập thẻ điểm, tự động nâng cấp chấp nhận để đánh bóng/viết lại
- Kiểm tra `save_review.ContractStatus`: hợp đồng=bị bỏ lỡ được tự động nâng cấp để viết lại
- `commit_chapter.CheckArcBoundary()`: Tính `arc_end / needs_expansion / needs_new_volume` nhanh chóng
- `commit_chapter.applyCompletion()`: Phán đoán tức thời `book_complete`
- `CommitResult` trả về 17 trường dữ kiện

**Kết luận**: Lớp công cụ đã mã hóa hầu hết các "đánh giá" và các quyết định do Điều phối viên đưa ra dựa trên những dữ kiện này về cơ bản là if-else.

### 2.4 Chi phí thực tế của hiện trạng

Điều phối viên LLM vòng mỗi chương:
- **1-2 lượt mỗi chương** (đọc lời nhắc hệ thống ~3000 mã thông báo + lời nhắc ~200 mã thông báo + lịch sử + CommitResult ~500 mã thông báo → tạo tool_call ~50 mã thông báo)
- Truyện dài 200 chương khoảng **200-400 lượt** Điều phối viên LLM gọi
- **~90% trong số đó là định tuyến thuần túy** (lời nhắc lặp lại LLM), **~10% là phán quyết**

**~3500-7000 token mỗi chương dành cho các quyết định của Điều phối viên, dư thừa 95%** (Lời nhắc đã tìm ra câu trả lời).

---

## 3. Phương án thiết kế: Điều phối viên lai

### 3.1 Ý tưởng cốt lõi

**Chuyển các quyết định xử lý từ LLM sang Máy chủ, nhưng giữ lại Điều phối viên làm nút trọng tài và kênh thực thi lệnh**.

```
┌──────────────────────────────────────────────────────────┐
│                   Entry (TUI / headless)                   │
└────────────────────────────────┬─────────────────────────┘
                                 │ Start / Resume / Steer
┌────────────────────────────────▼─────────────────────────┐
│                            Host                            │
│                                                             │
│   ┌──────────────────────────────────────────────────┐     │
│   │  Flow Router(lõi mới)                           │     │
│   │  ───────────                                      │     │
│   │  đăng ký Coordinator sự kiện:subagent tool Kích hoạt khi quay trở lại    │     │
│   │  Hàm thuần túy:route(Progress, Checkpoint, Boundary)     │     │
│   │      → NextInstruction                             │     │
│   │  Có hướng dẫn → coordinator.FollowUp(chỉ dẫn)                │     │
│   │  Không có hướng dẫn (tình huống xét xử)→ Đừng can thiệp, hãy để LLM quyền tự chủ            │     │
│   └──────────────────────────────────────────────────┘     │
│                                                             │
│   Thời gian lưu giữ: trọn đời API / Observer / Usage Tracker             │
│   dự trữ:resume.go(Logic cốt lõi được đơn giản hóa, không thay đổi)                       │
└────────────────────────────────┬─────────────────────────┘
                                 │
┌────────────────────────────────▼─────────────────────────┐
│                    Coordinator Agent (LLM)                  │
│                                                             │
│   Trách nhiệm được thu hẹp thành hai loại:                                             │
│   1. tiếp quản Host FollowUp chỉ dẫn → Tạo thư từ tool_call             │
│   2. người dùng Steer Tùy ý khi đến nơi (kiểm tra/Sửa đổi đánh giá)                  │
│                                                             │
│   coordinator.md: 88 ĐƯỢC RỒI → ~25 ĐƯỢC RỒI                             │
│   MaxTurns: 1000 Dành riêng (phản hồi cho người dùng steer + thực hiện Host chỉ dẫn)      │
└────────────────────────────────┬─────────────────────────┘
                                 │
                                 ▼
         ┌──────────────────────┼───────────────────────┐
         ▼                      ▼                       ▼
    ┌────────┐             ┌────────┐             ┌────────┐
    │Architect│             │ Writer │             │ Editor │
    └────────┘             └────────┘             └────────┘
```

### 3.2 Phân chia lại trách nhiệm

| Lớp | Phải làm gì | Không nên làm gì |
|---|---|---|
| **Bộ định tuyến máy chủ / luồng** | Đọc dữ kiện → Định tuyến hàm thuần túy → Lệnh FollowUp | Tự điều chỉnh SubAgent (vẫn thông qua Điều phối viên) |
| **Điều phối viên** | Thực thi hướng dẫn của Máy chủ + phân xử sự can thiệp của người dùng + chọn người lập kế hoạch khi khởi động | Đưa ra quyết định độc lập về "việc cần làm tiếp theo" |
| **Đại lý phụ (A/W/E)** | Công việc tương ứng của họ | Không có thay đổi |
| **Lớp công cụ** | Vị trí nguyên tử + thực tế trả về | Không thay đổi |

**Các bất biến chính**:
- ✅ Điều phối viên vẫn là Agent điều hành liên tục, giữ nguyên “nhận thức liên tục” về toàn bộ cuốn sách
- ✅ User Steer vẫn vượt `coordinator.Inject()`, gián đoạn ngay việc duy trì khả năng
- ✅ SubAgentTool vẫn được LLM gọi (lấy đường dẫn gốc của Agentcore) và luồng sự kiện/ContextManager/chuyển đổi mô hình không thay đổi.
- ✅ Agentcore không sửa đổi

### 3.3 Logic cụ thể của Flow Router

```go
// internal/host/flow/router.go

type NextInstruction struct {
    Agent  string   // architect_long / architect_short / writer / editor
    Task   string   // Mô tả nhiệm vụ cho đại lý phụ
    Reason string   // Đưa cho Coordinator Lý do xem (tùy chọn, thuận tiện cho việc gỡ lỗi)
}

type RouterState struct {
    Progress        *domain.Progress
    LatestCheckpoint *domain.Checkpoint
    // Ranh giới cung cho chế độ phân lớp (được tính khi hoàn thành chương trước)
    LastCompleted   int
    ArcBoundary     *store.ArcBoundary
    HasArcReview    bool
    HasArcSummary   bool
    // Thiếu cài đặt cơ bản
    FoundationMissing []string
}

// Route trả về bước tiếp theo của hướng dẫn. Trả về nil có nghĩa là để Điều phối viên tự đưa ra quyết định (kịch bản quyết định).
func Route(s RouterState) *NextInstruction {
    p := s.Progress

    // 0. Trạng thái cuối cùng: để LLM xuất bản tóm tắt mà không cần định tuyến
    if p.Phase == domain.PhaseComplete {
        return nil
    }

    // 1. Giai đoạn lập kế hoạch: Việc ra quyết định (lựa chọn người lập kế hoạch) được thực hiện bởi LLM, không định tuyến
    if p.Phase != domain.PhaseWriting {
        return nil
    }

    // 2. Giai đoạn viết
    // 2a. Ưu tiên hàng đợi viết lại/đánh bóng
    if len(p.PendingRewrites) > 0 {
        ch := p.PendingRewrites[0]
        verb := "viết lại"
        if p.Flow == domain.FlowPolishing {
            verb = "đánh bóng"
        }
        return &NextInstruction{
            Agent:  "writer",
            Task:   fmt.Sprintf("%sKHÔNG. %d chương", verb, ch),
            Reason: fmt.Sprintf("PendingRewrites hàng đợi còn lại %d chương", len(p.PendingRewrites)),
        }
    }

    // 2b. Đang xem xét: Không định tuyến, hãy để Điều phối viên đưa ra phán quyết dựa trên kết quả save_review.
    if p.Flow == domain.FlowReviewing {
        return nil
    }

    // 2c. Xử lý hậu kỳ cuối cung ở chế độ phân lớp
    if p.Layered && s.ArcBoundary != nil && s.ArcBoundary.IsArcEnd {
        b := s.ArcBoundary
        if !s.HasArcReview {
            return &NextInstruction{
                Agent:  "editor",
                Task:   fmt.Sprintf("Phải %d Âm lượng %d Arc thực hiện đánh giá cấp độ hồ quang", b.Volume, b.Arc),
                Reason: "Đánh giá cuối phần chưa hoàn thành",
            }
        }
        if !s.HasArcSummary {
            return &NextInstruction{
                Agent:  "editor",
                Task:   fmt.Sprintf("Tạo ra %d Âm lượng %d tóm tắt vòng cung", b.Volume, b.Arc),
                Reason: "Tóm tắt Arc chưa hoàn thành",
            }
        }
        if b.NeedsExpansion {
            return &NextInstruction{
                Agent:  "architect_long",
                Task:   fmt.Sprintf("Mở rộng chương %d Âm lượng %d cung(save_foundation type=expand_arc）", b.NextVolume, b.NextArc),
                Reason: "Bộ xương vòng cung tiếp theo sẽ được mở rộng",
            }
        }
        if b.NeedsNewVolume {
            return &NextInstruction{
                Agent:  "architect_long",
                Task:   "Đánh giá và thực hiện save_foundation(type=append_volume) hoặc mark_final",
                Reason: "Cuối tập phải có quyết định thêm tập mới",
            }
        }
    }

    // 2d. Tiếp tục bình thường
    next := p.NextChapter()
    return &NextInstruction{
        Agent:  "writer",
        Task:   fmt.Sprintf("viết cái đầu tiên %d chương", next),
        Reason: "Tiếp tục viết",
    }
}
```

**Tính năng chức năng**:
- Hàm thuần túy (đầu vào RouterState, đầu ra NextInstruction)
- Có thể kiểm tra một lần (trạng thái nhất định, xác nhận kết quả định tuyến)
- **Trả về nil là hợp pháp** - có nghĩa là "Đây là kịch bản có tính quyết định, vui lòng để LLM tự chủ"

### 3.4 Thời gian kích hoạt

Máy chủ đăng ký các sự kiện `agentcore.EventToolExecEnd`:

```go
coordinator.Subscribe(func(ev agentcore.Event) {
    if ev.Type == agentcore.EventToolExecEnd && ev.Tool == "subagent" && !ev.IsError {
        // SubAgent vừa quay lại → đọc trạng thái mới nhất → định tuyến
        h.flowRouter.Dispatch()
    }
})
```

```go
func (r *FlowRouter) Dispatch() {
    state := r.loadState()
    instruction := Route(state)
    if instruction == nil {
        return // Kịch bản xét xử, hãy LLM quyền tự chủ
    }
    msg := formatInstruction(instruction)
    _ = r.coordinator.FollowUp(agentcore.UserMsg(msg))
}

func formatInstruction(i *NextInstruction) string {
    return fmt.Sprintf(
        "[Host Ra lệnh] Bước tiếp theo: gọi subagent(%s, %q)\n"+
        "lý do:%s\n"+
        "Đây là hướng dẫn rõ ràng từ lớp quy trình. Hãy thực hiện nó ngay lập tức. Đừng điều chỉnh nó đầu tiên. novel_context, không đưa ra suy luận trước.",
        i.Agent, i.Task, i.Reason,
    )
}
```

### 3.5 Khả năng đáp ứng và đồng thời

**Đường dẫn chỉ đạo người dùng** (không thay đổi):
```
Steer → coordinator.Inject(UserMsg("[sự can thiệp của người dùng] xxx"))
```

- Đang chạy: Tin nhắn được chèn vào hàng đợi chạy hiện tại
- Idle：resume run
- Tạm dừng: xếp hàng

**Chỉ thị định tuyến + Đồng thời chỉ đạo**:
- Tất cả vào hàng đợi tin nhắn của Điều phối viên và được xử lý theo thứ tự gốc của Agentcore.
- Nếu Host chỉ gửi `FollowUp("[Host chỉ dẫn] viết cái đầu tiên 37 chương")` thì người dùng Chỉ đạo `"Dừng lại và điều chỉnh phong cách của bạn"`
  - Điều phối viên xử lý hướng dẫn của Máy chủ trước? Hay xử lý Steer trước?
  - **Ngữ nghĩa của `Inject` là xếp hàng về đầu hàng đợi hiện tại** nên Steer được xử lý trước
  - Đây là hành vi được mong đợi: sự can thiệp của người dùng được ưu tiên hơn so với việc lập lịch trình thường xuyên của Máy chủ

**Tránh xung đột giữa chỉ thị của Máy chủ và Chỉ đạo**:
- Bộ định tuyến luồng **tạm dừng** một thời gian ngắn sau khi nhận được tín hiệu "Chỉ đạo đã được tiêm" trong vài lượt (để Điều phối viên xử lý xong Chỉ đạo trước khi định tuyến)
- Kiểm tra sự thay đổi trạng thái tiến trình Chỉ đạo kết quả xử lý bằng cách đăng ký `agentcore.EventMessageEnd` +

### Ví dụ đơn giản hóa 3.6 coctor.md

Cắt từ 88 dòng xuống còn khoảng 25 dòng:

```markdown
Bạn là người điều phối chung của việc sáng tạo tiểu thuyết.

##chế độ làm việc của bạn

**Dòng chính**：Host Sẽ được giải phóng mỗi khi đại lý phụ quay trở lại `[Host Ra lệnh]` Thông báo cho bạn biết cần gọi đại lý phụ nào tiếp theo để làm gì. Tạo phản hồi ngay sau khi nhận được hướng dẫn tool_call, không điều chỉnh trước novel_context Lý do, đừng trình bày lại.

**cầm quyền**: Bạn cần tự mình đưa ra phán đoán khi gặp những tình huống sau (Host Sẽ không ra lệnh, bạn phải chủ động):

### Lúc khởi động: chọn Planner

- mặc định → `architect_long`
- Chỉ khi người dùng yêu cầu một câu chuyện ngắn một cách rõ ràng/tập đơn/Bản phác thảo bị giới hạn về chiều dài 25 trong chương → `architect_short`

Chẳng hạn như đầu vào của người dùng < 20 từ, đầu tiên task Thêm hướng đi khác biệt, độc giả mục tiêu và ít nhất một câu chuyện độc đáo vào phần mô tả trước khi phân phối nó.

### Chỉ đạo người dùng

Định dạng:`[sự can thiệp của người dùng] xxx`

- **Lớp truy vấn**( hỏi trạng thái/Cài đặt): Xuất trực tiếp câu trả lời bằng văn bản, không cần điều chỉnh công cụ;Host Việc phân phối sẽ tiếp tục.
- **Sửa đổi lớp**(Yêu cầu thay đổi cài đặt/viết lại/Điều chỉnh phong cách): Đánh giá phạm vi ảnh hưởng:
  - Liên quan đến thay đổi cài đặt → điều chỉnh architect_* LÀM `save_foundation(type=...)`
  - Liên quan đến các chương đã được viết → Để công cụ tự động viết chương mục tiêu `PendingRewrites`(Có thể điều chỉnh lại bằng cách writer khi chỉ ra ý định viết lại)
  - Chỉ ảnh hưởng đến các kiểu tiếp theo → Mô tả ngắn gọn yêu cầu của bạn và tôi sẽ nhận được nó vào lần sau Host chỉ thị khi được thêm vào writer của task trong mô tả

## dụng cụ

- `subagent(agent, task)`: Gọi đại lý phụ
- `novel_context`: Chỉ sử dụng khi người dùng yêu cầu truy vấn, không sử dụng Host Sau khi có hướng dẫn, hãy điều chỉnh nó trước

## đại lý phụ

- `architect_long` / `architect_short` / `writer` / `editor`

## cấm

- hiện hữu Host Được gọi đầu tiên khi có lệnh đến novel_context hành động lại
- Không có người dùng Steer và không Host Quyết định bước tiếp theo của riêng bạn trong trường hợp được hướng dẫn
```

### 3.7 Kênh nhắc nhở được thu gọn đáng kể

**xóa bỏ**:
- `flow.go` (Host FollowUp đã đưa ra hướng dẫn cụ thể, nhắc nhở định tuyến của Reminder mất giá trị)
- `queue_guard.go` (hàng đợi được đảm bảo bởi Bộ định tuyến máy chủ)
- `book_complete.go` (Lệnh tóm tắt đầu ra FollowUp khi Máy chủ ở Giai đoạn=Hoàn thành)

**dự trữ**:
- `subagent_guards.go` (StopGuard dành cho Nhà văn/Kiến trúc sư/Biên tập viên, đảm bảo các tác nhân phụ không trở về tay trắng)
- Đã thêm `foundation_reminder.go` nhẹ: giai đoạn lập kế hoạch thông báo cho Điều phối viên các mục còn thiếu (đây là thông tin cần thiết cho phán quyết chứ không phải hướng dẫn định tuyến)

**Dành riêng cho StopGuard**:
- StopGuard của điều phối viên được bảo lưu (end_turn được sử dụng làm vỏ bọc khi sử dụng `Phase != Complete`)
- Đã thêm lời nhắc tiêm khi "Nhận được lệnh máy chủ nhưng tác nhân phụ tương ứng không được điều chỉnh trong vòng này"

### 3.8 sơ yếu lý lịch. Đơn giản hóa một chút

Hiện tại, `buildResumePrompt` tạo hướng dẫn ngôn ngữ tự nhiên chính xác đến từng điểm kiểm tra (dòng 121).

Kiến trúc mới:
- Đọc tiến trình đầu tiên khi Resume, và Flow Router tính toán `NextInstruction`
- Điều phối viên nhận được lời nhắc tiếp tục **rất ngắn gọn** và sau đó chờ lệnh Theo dõi của Chủ nhà.

```
[hồi phục] Cuốn sách này"xxx"Hoàn thành N Chương, nhập XX sân khấu.
Vui lòng chờ Host Hướng dẫn tiếp theo hoặc xử lý sự can thiệp của người dùng có thể còn sót lại trong thời gian ngừng hoạt động.
```

Hầu hết tất cả logic nhánh đều chuyển sang Flow Router (Bộ định tuyến vốn được định tuyến theo trạng thái và Resume không yêu cầu đường dẫn đặc biệt).

---

## 4. Đánh giá việc đạt được mục tiêu

### 4.1 Tính ổn định

| Rủi ro | Hiện tại | Kiến trúc mới |
|---|---|---|
| Lựa chọn sai kiến ​​trúc sư Điều phối viên | Đã xảy ra (lỗi định tuyến giữa) | Nó vẫn có hiệu lực khi bắt đầu, nhưng lời nhắc thay đổi từ ba thành hai (đã xong), vùng lỗi giảm đi rất nhiều |
| Điều phối viên không tuân thủ “Chỉ nói và viết Chương N” | Đã xảy ra | Máy chủ đã đưa ra hướng dẫn định dạng cố định, không cần LLM để tạo mô tả nhiệm vụ nữa |
| Điều phối viên đã bỏ lỡ kiểm tra queue_drained | Đã xảy ra | Host Router buộc phải đi theo thứ tự |
| Điều phối viên quên điều chỉnh trình soạn thảo sau khi xác nhận ở cuối phần | Có thể | Bộ định tuyến máy chủ đã phát hiện IsArcEnd && !HasArcReview và gửi nó trực tiếp |
| Nhánh phục hồi sự cố bị thiếu | Khoảng trống đã biết | Máy trạng thái của Flow Router tự động bao phủ tất cả các nhánh |
| StopGuard chặn 5 lần nâng cấp liên tiếp gây tử vong | Tồn tại | Sau khi lệnh Host rõ ràng, LLM khó có thể chặn liên tục (trừ khi nhắc lỗi nghiêm trọng) |

### 4.2 Phần thưởng nâng cấp LLM

| Kích thước | Giữ chân |
|---|---|
| Nâng cấp mẫu máy viết → Chất lượng viết | 100% |
| Nâng cấp mô hình biên tập → Đánh giá chính xác | 100% |
| Nâng cấp mô hình kiến ​​trúc → Quy hoạch tinh tế | 100% |
| **Nâng cấp mô hình điều phối → phán quyết chính xác hơn** | **100%** (cảnh cầm quyền được giữ lại)|
| ~~Nâng cấp mô hình điều phối → định tuyến chính xác hơn~~ | Bỏ cuộc (tỷ lệ lỗi định tuyến phải bằng 0, không cần LLM trở nên thông minh hơn) |

**Bảo lưu quan trọng**: Các tình huống phân xử như đánh giá sự can thiệp của người dùng, lựa chọn người lập kế hoạch và phán đoán ranh giới phán quyết vẫn do LLM xử lý, được hưởng lợi trực tiếp từ việc nâng cấp mô hình.

### 4.3 Khả năng đa tác nhân

- Số lượng, chức năng, cách thức lắp ráp SubAgent **hoàn toàn không thay đổi**
- Tính không đồng nhất của mô hình (cấu hình độc lập của người điều phối/kiến trúc sư/người viết/biên tập viên) **hoàn toàn không thay đổi**
- Điều phối viên vẫn chạy liên tục, giữ nguyên “góc nhìn toàn sách”
- Phương tiện cộng tác (sản phẩm trong Store) không thay đổi

### 4.4 Khả năng phản hồi

- Khả năng ngắt thông qua `coordinator.Inject` của Người điều khiển người dùng **được giữ lại hoàn toàn**
- Bộ định tuyến máy chủ gửi hướng dẫn khi SubAgent quay lại và sử dụng kênh thông báo giống như người dùng Steer
- Inject có mức độ ưu tiên cao hơn FollowUp (ngữ nghĩa của `Inject` là nhảy vào hàng), Steer sẽ không bị lệnh Host ép ra

### 4,5 Giá token

Chương hiện tại: Điều phối viên ~3500-7000 token × 1-2 lượt = 3500-14000 token

Mỗi chương của kiến ​​trúc mới:
- Lời nhắc của điều phối viên giảm từ ~3000 mã thông báo xuống ~800 mã thông báo
- Mỗi chương vẫn cần 1 lượt (Điều phối viên đọc lệnh FollowUp + tạo tool_call)
- Tổng cộng ~1000-1500 token

**Tiết kiệm 60-80%**. 200 chương dài tiết kiệm được khoảng 400 nghìn-1 triệu mã thông báo (không tốt bằng 100% giải pháp triệt để nhưng không ảnh hưởng đến khả năng phản hồi và phối cảnh cuốn sách đầy đủ).

---

## 5. Tác động tới docs/architecture.md

### 5.1 §2 Điều chỉnh nguyên tắc cốt lõi

**Nguyên tắc 1** (Vòng lặp chính điều khiển LLM) → Điều chỉnh thành:
```
LLM Thúc đẩy việc sáng tạo và ra quyết định,Host Định tuyến quá trình ổ đĩa.

- Việc sáng tạo và xét xử (các quyết định đòi hỏi sự hiểu biết về ngữ nghĩa, đánh giá chất lượng và nhận biết mục đích) vẫn được giao cho LLM
- Định tuyến quy trình (Đọc sự kiện→Tra cứu bảng→hướng dẫn ban hành) bởi Host Trách nhiệm về mã
- Host Đừng bỏ qua Coordinator Điều chỉnh trực tiếp SubAgent, nhưng thông qua FollowUp Đưa ra hướng dẫn rõ ràng,
  dự trữ Coordinator Là kênh thực hiện lệnh và nút trọng tài
```

**Nguyên tắc 2** (Đặt cược vào khả năng của mô hình, không phải vào mã hóa cứng) → Điều chỉnh thành:
```
Đặt cược vào các mô hình trong các khía cạnh sáng tạo và xét xử (Writer/Editor/Architect/Coordinator khả năng xét xử),
Được thể hiện bằng mã theo chiều định tuyến quy trình (danh mục dọc agent Không gian quyết định được đóng lại và tác vụ tra cứu bảng LLM Không có tiền thưởng).
```

### 5.2 §13 Điều chỉnh danh sách bị cấm

- §13.13 "Không đọc tệp tín hiệu bằng Máy chủ → Đưa vào mặt phẳng điều khiển xác định của lệnh tiếp theo" →
  **Từ ngữ đã sửa**: "Không cần thực hiện IPC trong tệp tín hiệu (chỉ cần đọc trực tiếp Tiến trình + Điểm kiểm tra). Máy chủ đọc thông tin thực tế và sau đó đưa ra hướng dẫn gọi đại lý phụ rõ ràng thông qua `coordinator.FollowUp`, đây là một định tuyến dọc hợp lý."
- §13.14 "Không di chuyển luồng máy trạng thái được mã hóa cứng" →
  **Từ ngữ đã được sửa**: "Thẻ luồng vẫn chỉ được công cụ cập nhật (không có máy trạng thái nào trong Máy chủ ghi 'nếu A thì SetFlow(B)'), nhưng Bộ định tuyến luồng có thể quyết định ai sẽ gọi tiếp theo dựa trên Luồng và các dữ kiện khác."

### 5.3 §7 Điều chỉnh cụm đại lý

- Giữ điều phối viên
- `coordinator.md` cắt từ 88 dòng xuống còn ~25 dòng
- Giảm kênh nhắc nhở (xóa flow/queue_guard/book_complete, giữ lại nền tảng/subagent_guards)
- Đã thêm gói `internal/host/flow/`

---

## 6. Điểm yếu đã biết (danh sách trung thực)

### 6.1 Sự phát triển lâu dài của Flow Router

- Khi các cảnh mới được thêm vào (trạng thái luồng mới, xử lý hậu kỳ vòng cung mới), trường hợp chuyển đổi của Bộ định tuyến sẽ dài hơn
- Yêu cầu các ràng buộc nghiêm ngặt: **Chỉ xử lý định tuyến, không phải logic nghiệp vụ**; viết các bài kiểm tra đơn lẻ cho các quy tắc ra quyết định
- Các cảnh báo tương tự v0.0.1 `handleSubAgentDone` luôn hợp lệ; nhưng giải pháp này sử dụng "hàm thuần túy + ​​kiểm tra đơn lẻ + chỉ gọi các sự kiện thuần túy" để tránh trượt vào các đối tượng của Chúa

### 6.2 Mức độ phức tạp của sự can thiệp của người dùng

- Thiết kế hiện tại hoàn toàn phụ thuộc vào quyết định LLM của Điều phối viên
- Nhưng một số Chỉ đạo trải rộng trên nhiều danh mục (chẳng hạn như "Thay đổi một vài chương đầu tiên của nhân vật A + thêm dòng nhánh cho anh ta sau")
- Cần dựa vào khả năng tháo dỡ của LLM, cần kịp thời hướng dẫn rõ ràng
- **Phần nâng cấp mô hình này trực tiếp mang lại lợi ích** (So với phân loại enum được mã hóa cứng của InterventionAgent, khả năng ra quyết định linh hoạt của LLM phù hợp hơn với các tình huống thực tế)

### 6.3 Sự phụ thuộc trước vào tính nhất quán của lớp thực tế

- Bộ định tuyến đưa ra quyết định dựa trên Tiến trình + Điểm kiểm tra và lớp thực tế phải đáng tin cậy
- Gói `withWriteLock` hiện tại tốt và bộ commit_chapter gồm ba phần đã được hoàn thiện cơ bản.
- Nhưng nếu có sự không nhất quán trong lớp thực tế (ví dụ: Progress nói Chương 3 đã hoàn thành nhưng các chương/ thì chưa), Router sẽ đưa ra quyết định sai lầm
- Đề xuất: Thêm **kiểm tra tính nhất quán của lớp thực tế** khi khởi động (nếu phát hiện Progress.CompletedChapters không khớp với thư mục chương/, một cảnh báo sẽ được báo cáo)

### Điều phối viên 6.4 vẫn giữ khả năng định tuyến LLM

- Ngay cả khi hướng dẫn rõ ràng, LLM có thể không thực hiện chúng một cách "sáng tạo" (ví dụ: sau khi tạo văn bản tư duy và sau đó gọi công cụ)
- StopGuard: Khi nhận được lệnh Host nhưng tác nhân phụ không được điều chỉnh trong vòng này, một lời nhắc sẽ được đưa vào
- Đây là cảnh báo chứ không phải cấm đoán - việc người mẫu mạnh thỉnh thoảng “nghĩ thêm một bước” cũng không phải là điều xấu

### 6.5 Cải thiện các yêu cầu về phạm vi kiểm tra

- Flow Router là một hàm thuần túy và phải có một thử nghiệm đơn hoàn chỉnh (bao gồm tất cả các kết hợp Pha × Luồng × Biên)
- Kiểm tra tích hợp: mô phỏng liên kết đầy đủ của "cam kết → bộ định tuyến → FollowUp → phản hồi của điều phối viên → đại lý phụ"
- Kiểm tra khôi phục sự cố: tắt tiến trình rồi tiếp tục, xác nhận rằng Bộ định tuyến thực hiện đúng bước tiếp theo

---

## 7. Lộ trình thực hiện

### Giai đoạn 1: Tăng cường lớp sự kiện (khoảng 0,5 ngày)

- Hoàn thành kiểm tra tính nhất quán trong §6.3: quét một lần khi khởi động/Tiếp tục và tạo cảnh báo
- Đảm bảo có sẵn API `store.HasArcReview(vol, arc)` và `HasArcSummary(vol, arc)` (thêm chúng nếu không)

### Giai đoạn 2: Giới thiệu bộ khung Flow Router (khoảng 1 ngày)

- Tạo gói `internal/host/flow/` mới:
  - `route.go` — hàm thuần túy `Route(state) → *NextInstruction`
  - `dispatcher.go` — Sự kiện đăng ký + Phát hành theo dõi
  - `route_test.go` — thử nghiệm đơn bao gồm tất cả các nhánh
- Điều khiển kích hoạt thông qua config switch `flow_driven: true/false`
- Mặc định tắt (false), chạy so sánh trước

### Giai đoạn 3: Kích hoạt và xác minh (khoảng 1 ngày)

- Mở `flow_driven: true`
- Chạy một tiểu thuyết 30-50 chương và so sánh các chỉ số:
  - Số cuộc gọi Điều phối viên LLM
  - Số lỗi định tuyến (nên là 0)
  - Khả năng phản hồi (liệu sự gián đoạn của người quản lý có bình thường hay không)
- Sửa lỗi và điều chỉnh quy tắc Router

### Giai đoạn 4: đơn giản hóa Coctor.md + Nhắc nhở giảm béo (khoảng 0,5 ngày)

- Thay đổi Coventor.md theo §3.6
- Loại bỏ `reminder/flow.go / queue_guard.go / book_complete.go`
- Giữ lời nhắc nền tảng cần thiết
- Cập nhật StopGuard của tác nhân phụ nếu cần thiết (thường không bắt buộc)

### Giai đoạn 5: Đơn giản hóa sơ yếu lý lịch (~0,5 ngày)

- Loại bỏ hầu hết các nhánh của `buildResumePrompt`
- Được thay thế bằng thông báo ngắn chung chung "[Khôi phục] Vui lòng đợi lệnh của Máy chủ"
- Sau khi Resume thì Router sẽ tự động suy ra hành động tiếp theo.

### Giai đoạn 6: Cập nhật tài liệu kiến ​​trúc (~0,5 ngày)

- Nhấn §5 để sửa đổi `docs/architecture.md` §2/§13/§7
- Thay đổi trạng thái tài liệu của đề xuất này thành "Đã thông qua" và lưu trữ vào `docs/history/`

### Giai đoạn 7: Thời gian quan sát (2-4 tuần)

- Chạy 2-3 tiểu thuyết liên tiếp (mỗi chương hơn 100 chương)
- Ghi lại tất cả các lỗi định tuyến (nếu có), các vấn đề về phản hồi, hành vi Điều phối viên không mong muốn
- Tinh chỉnh các quy tắc Bộ định tuyến và điều phối viên.md dựa trên các quan sát

**Tổng thời gian thực hiện + thời gian quan sát khoảng 4 ngày**.

---

## 8. Bảng so sánh

| Kích thước | Kiến trúc hiện tại | Lai (kế hoạch này) | Kế hoạch căn cơ (Phụ lục A) |
|---|---|---|---|
| Tính ổn định | Trung bình (LLM đôi khi định tuyến không chính xác) | **Cao** | Cao |
| Khả năng đáp ứng | Cao | **Cao** | **Thấp** (Host trực tiếp điều chỉnh SubAgent và không thể ngắt) |
| Tiền thưởng LLM | 100% | **100%** | 85% (Miễn kích thước định tuyến) |
| Tiết kiệm mã thông báo | 0 | ~70% | ~95% |
| Quan điểm cuốn sách đầy đủ | Có | **Có** | Không (SubAgent độc lập mỗi lần) |
| Chi phí thực hiện | - | Trung bình (~4 ngày) | Cao (~1 tuần + thay đổi lõi tác nhân) |
| Cập nhật tài liệu | - | Nhỏ (điều chỉnh §2/§13) | Lớn (§2 nguyên tắc viết lại) |
| Cần thay đổi Agentcore | - | Không | Có thể (điều chỉnh trực tiếp SubAgent) |
| Khó khăn quay trở lại | - | thấp (chuyển đổi cấu hình) | cao |

---

## 9. Điểm quyết định

1. **Bạn có muốn áp dụng đề xuất này (Điều phối viên kết hợp) không? ** [ ] Đã thông qua · [ ] Đã thông qua sau khi sửa đổi · [ ] Không được thông qua
2. Giai đoạn 3 có được triển khai và xác minh trước tiên với tư cách là PR độc lập không? [ ]
3. Lần này các điều chỉnh cho `docs/architecture.md` §2 / §13 có được xử lý cùng nhau không? [ ]
4. Độ dài thời gian quan sát: [ ] 2 tuần · [ ] 4 tuần · [ ] dài hơn

---

## Phụ lục A: Đã đánh giá các phương án cấp tiến (loại bỏ hoàn toàn Điều phối viên)

> Kế hoạch dự thảo đầu tiên. Bị hạ cấp xuống mức tham chiếu do các vấn đề như khả năng phản hồi kém, tính khả thi về mặt kỹ thuật có vấn đề và mất quan điểm trong toàn bộ cuốn sách Điều phối viên.

Cốt lõi của giải pháp triệt để: Host điều chỉnh trực tiếp `SubAgentTool.Execute` mà không cần thông qua CoĐiều phối viên LLM.

**Vấn đề đã được xác định**:

1. **Hồi quy khả năng phản hồi**: `SubAgentTool.Execute` là một cuộc gọi đồng bộ chặn và người dùng Chỉ đạo phải đợi Tác nhân phụ hiện tại quay lại trước khi xử lý nó. Kiến trúc hiện tại của `Inject` có thể bị phá vỡ ngay lập tức.
2. **Tính khả thi về mặt kỹ thuật còn nghi ngờ**:
   - Máy chủ gọi trực tiếp SubAgentTool, vi phạm thông lệ sử dụng Agentcore
   - Luồng sự kiện (Sự kiện `Subscribe`) có thể không hiển thị chính xác cho người quan sát
   - Đường dẫn gọi lại `ContextManagerFactory`/`OnMessage` của SubAgent không xác định
   - Cần thay đổi Agentcore hoặc thay đổi đáng kể người quan sát
3. **Mất phối cảnh toàn bộ cuốn sách của Điều phối viên**: Mỗi lần SubAgent chạy độc lập, không có "người theo dõi LLM liên tục". Các vấn đề như lệch phong cách và phân mảnh vai trò trong quá trình chạy đường dài đang thiếu một lớp bảo vệ vô hình.
4. **InterventionAgent được đơn giản hóa quá mức**: Giải pháp triệt để sử dụng enum (query/modify_setting/rewrite_chapters/ adjustment_style/noop) để phân loại ý định của người dùng. Chỉ đạo thực sự có thể trải rộng trên nhiều danh mục và lược đồ bắt buộc sẽ gây ra sự phân loại sai.
5. **Khối lượng công việc nặng nề khi viết lại tài liệu kiến ​​trúc**: §2 nguyên tắc cốt lõi bị đảo lộn và 30% nội dung thảo luận trong tài liệu bị ảnh hưởng.
6. **FlowDriver sẽ phát triển thành một đối tượng thần thánh**: Tất cả logic định tuyến bị chặn trong một vòng lặp, vòng lặp này phải được thay đổi mỗi khi cảnh được thêm vào. Nó đẳng cấu với v0.0.1 `handleSubAgentDone`.

Giải pháp Kết hợp tránh được bốn vấn đề đầu tiên, vấn đề thứ năm được giảm xuống mức tinh chỉnh và vấn đề thứ sáu được kiểm soát bằng "chức năng thuần túy + ​​thử nghiệm đơn lẻ".

---

## Phụ lục B: Chi tiết vị trí đặt điểm quyết định

| Điểm quyết định | Vị trí hiện tại | Vị trí kiến ​​trúc mới | Loại |
|---|---|---|---|
| Chọn người lập kế hoạch | điều phối viên.md L26-29 | Điều phối viên LLM phán quyết (khi khởi động) | cai trị |
| Tiện ích mở rộng đầu vào | điều phối viên.md L31 | Điều phối viên Trọng tài LLM (khi khởi động) | Trọng tài |
| Vòng hoàn thành kế hoạch | điều phối viên.md L36-38 | Giai đoạn bộ định tuyến máy chủ=Nhánh tiền đề/phác thảo (trả về nil đặt kiến ​​trúc FollowUp tự động hoặc rõ ràng của LLM) | Hỗn hợp |
| Bước tiếp theo mỗi chương | điều phối viên.md L46-51 + lời nhắc/luồng | **Chi nhánh Bộ định tuyến máy chủ 2d** (Người viết tiếp theo) | Định tuyến |
| Đánh giá kết thúc Arc | điều phối viên.md L78-82 | **Chi nhánh Bộ định tuyến máy chủ 2c** (Trình soạn thảo/kiến trúc sư FollowUp) | Định tuyến |
| nĩa phán quyết | điều phối viên.md L59-61 + công cụ save_review | Lớp công cụ đã được mã hóa, Router chỉ đọc Flow | Định tuyến (đã hoàn thành) |
| Sự can thiệp của người dùng | điều phối viên.md L67-70 | Điều phối viên xét xử LLM (khi nhận được tin nhắn Tiêm) | xét xử |
| Lỗi lập kế hoạch sắp xếp lại | điều phối viên.md L40 | Bộ định tuyến máy chủ phát hiện FoundationMissing không thay đổi, hãy thử lại | Định tuyến |
| Tóm tắt hoàn thành cuốn sách | điều phối viên.md L63-65 + nhắc nhở/book_complete | Giai đoạn phát hiện bộ định tuyến máy chủ=Hoàn thành → Theo dõi "Tóm tắt đầu ra" | Định tuyến |

---

## Phụ lục C: Vị trí mã nguồn tham khảo

- `assets/prompts/coordinator.md` - được đơn giản hóa
- `internal/host/reminder/flow.go`/`queue_guard.go`/`book_complete.go` — sẽ bị xóa
- `internal/host/reminder/subagent_guards.go` - dành riêng
- `internal/host/reminder/stop_guard.go` — dành riêng + thêm kiểm tra "lệnh máy chủ đã nhận phải được thực thi"
- `internal/host/resume.go` - đơn giản hóa rất nhiều
- `internal/host/observer.go` — Đăng ký mới Trình kích hoạt EventToolExecEnd Bộ định tuyến
- `internal/host/flow/` — Gói mới
- `internal/tools/commit_chapter.go` L220-280 — 17 trường CommitResult đã hoàn tất
- `internal/tools/save_review.go` L76-116 — Nâng cấp phán quyết và di chuyển luồng được mã hóa
- `internal/store/outline.go` `CheckArcBoundary` — API sự kiện ranh giới vòng cung
