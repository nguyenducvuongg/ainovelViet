# ainovel-cli

Công cụ tạo tiểu thuyết AI hoàn toàn tự động. Điều phối viên thúc đẩy ba tác nhân phụ là Kiến trúc sư/Nhà văn/Biên tập viên hoàn thành việc tạo toàn bộ cuốn sách trong một Lời nhắc, trong khi Máy chủ chỉ thực hiện khởi động, khôi phục và quan sát. Từ một yêu cầu câu đến một cuốn tiểu thuyết hoàn chỉnh, toàn bộ quá trình không cần sự can thiệp của con người.

<p align="center">
  <img src="scripts/sample.gif" alt="ainovel-cli demo" width="800">
  <img src="scripts/novel.png" alt="ainovel-cli bg" width="800">
</p>

## Đặc trưng

- **Cộng tác giữa nhiều tác nhân** — Điều phối viên lên lịch cho ba tác nhân phụ Kiến trúc sư / Người viết / Biên tập viên trong một vòng lặp dài để đưa ra quyết định độc lập trong quá trình sáng tạo
- **Vòng lặp dài do LLM điều khiển** — Viết toàn bộ cuốn sách trong một lời nhắc và Máy chủ không can thiệp vào việc lên lịch. Càng đơn giản, càng ổn định, bác bỏ những sắp xếp phức tạp
- **Khôi phục điểm dừng cấp độ** — ghi điểm kiểm tra sau khi thực hiện thành công từng công cụ, chính xác để lập kế hoạch/dự thảo/kiểm tra/cam kết khôi phục cấp độ bước sau sự cố
- **Lập kế hoạch cuộn hai lớp** — Truyện dài không còn lập kế hoạch cho tất cả các chương cùng một lúc. Ban đầu, chỉ có khung của 2 phần đầu tiên + các chương chi tiết của phần đầu tiên được lên kế hoạch. Các phần/tập tiếp theo sẽ được Kiến trúc sư mở rộng khi quá trình viết diễn ra. Mỗi bản mở rộng sẽ đề cập đến phần tóm tắt và trạng thái nhân vật trước đó nên việc lập kế hoạch dài hạn không hề trống rỗng.
- **Đề xuất thông minh về các chương liên quan** — Khi viết mỗi chương, các chương lịch sử có liên quan sẽ tự động được đề xuất từ ​​bốn khía cạnh báo trước, ngoại hình nhân vật, thay đổi trạng thái và các mối quan hệ, đồng thời khớp với phần xem trước của chương tiếp theo để đảm bảo tính liên tục của cuốn tiểu thuyết dài hơn 500 chương.
- **Chiến lược ngữ cảnh thích ứng** — Tự động chuyển đổi giữa cửa sổ đầy đủ/trượt/tóm tắt phân cấp dựa trên tổng số chương, hỗ trợ hơn 500 chương
- **Bảy khía cạnh của đánh giá chất lượng** — Đánh giá của biên tập viên từ bảy khía cạnh: tính nhất quán trong thiết lập, hành vi nhân vật, nhịp điệu, sự mạch lạc của câu chuyện, điềm báo, điểm hấp dẫn và chất lượng thẩm mỹ. Các khía cạnh thẩm mỹ được chia thành năm loại: kết cấu miêu tả/kỹ thuật kể chuyện/phân biệt đối thoại/chất lượng ngôn từ/tác động cảm xúc. Mỗi mục phải trích dẫn văn bản gốc làm bằng chứng.
- **Can thiệp người dùng theo thời gian thực** — Đưa nhận xét sửa đổi vào hộp nhập liệu bất cứ lúc nào trong quá trình viết (không cần tạm dừng), hệ thống tự động đánh giá phạm vi ảnh hưởng và viết lại các chương bị ảnh hưởng
- **Cổng TUI hợp nhất** — Giao diện tương tác cho phép bạn quan sát tiến trình trong thời gian thực và cũng hỗ trợ bắt đầu trực tiếp theo yêu cầu.
- **Hỗ trợ nhiều LLM** — OpenRouter / Anthropic / Gemini / OpenAI, v.v. chuyển đổi theo ý muốn

## Ngành kiến ​​​​trúc

Thiết kế cốt lõi: **Trình điều khiển LLM, Dịch vụ máy chủ**. Điều phối viên xác định độc lập toàn bộ quá trình tạo sách trong một lần chạy, trong khi Máy chủ chỉ thực hiện khởi động, khôi phục và quan sát sự kiện.

```
┌─────────────────────────────────────────────────┐
│                Host(hộp mỏng)                     │
│           khởi động / hồi phục / quan sát / tiêm can thiệp            │
└──────────────────────┬──────────────────────────┘
                       │ một lần Prompt
┌──────────────────────▼──────────────────────────┐
│              Coordinator（LLM vòng lặp dài)            │
│    đọc novel_context → Chất giai điệu → đọc kết quả → Tiếp tục     │
└────┬──────────┬──────────┬──────────────────────┘
     │          │          │
 ┌───▼────┐ ┌───▼───┐ ┌────▼────┐
 │Architect│ │Writer │ │ Editor  │
 └───┬────┘ └───┬───┘ └────┬────┘
     └──────────┼──────────┘
                │ gọi công cụ (IO + checkpoint）
┌───────────────▼─────────────────────────────────┐
│                   Store                         │
│  Progress / Checkpoint / Outline / Drafts / ... │
└─────────────────────────────────────────────────┘
```

- **Máy chủ** — Khởi động Điều phối viên, khắc phục sự cố, chiếu sự kiện tới TUI. Không đưa ra bất kỳ quyết định lập kế hoạch nào
- **Điều phối viên** — Người ra quyết định duy nhất, thúc đẩy toàn bộ quá trình lập kế hoạch → viết → đánh giá → tóm tắt trong một lần chạy
- **Đại lý phụ** — Kiến trúc sư / Nhà văn / Biên tập viên có bối cảnh độc lập và cộng tác thông qua các tạo phẩm trong Cửa hàng
- **Công cụ** — Ghi điểm kiểm tra IO + nguyên tử, chỉ trả về JSON thực tế mà không có hướng dẫn

### Trách nhiệm của Đại lý

| Đại lý | Trách nhiệm | Công cụ |
|--------|------|------|
| **Điều phối viên** | Lập kế hoạch toàn cầu, xử lý các quyết định xem xét và can thiệp của người dùng | `subagent` `novel_context` |
| **Kiến trúc sư** | Tạo tiền đề, dàn ý, tiểu sử nhân vật, quy luật thế giới | `novel_context` `save_foundation` |
| **Nhà văn** | Hoàn thành việc lên ý tưởng, viết, tự nhận xét và nộp chương một cách độc lập | `novel_context` `read_chapter` `plan_chapter` `draft_chapter` `check_consistency` `commit_chapter` |
| **Biên tập viên** | Đọc văn bản gốc và xem lại nó từ cả cấp độ cấu trúc và thẩm mỹ | `novel_context` `read_chapter` `save_review` `save_arc_summary` `save_volume_summary` |

### Quy trình viết

```
Nhu cầu của người dùng → Architect Bộ khung quy hoạch + Chương cung đầu tiên → Writer viết từng chương → Editor đánh giá cấp độ vòng cung
                                                  ↑                   │
                                                  ├── viết lại/đánh bóng ◄──────┘
                                                  │
                                           Architect Mở rộng cung tiếp theo/cuộn
                                          (Tham khảo phần tóm tắt trước+ảnh chụp nhanh nhân vật)
```

Người viết hoàn thành mỗi chương theo một thứ tự cố định (nội dung viết hoàn toàn độc lập và thứ tự gọi công cụ rất nghiêm ngặt):

1. `novel_context` - Đang tải ngữ cảnh (tóm tắt trước, điềm báo, trạng thái nhân vật, quy tắc văn phong, đề xuất chương liên quan)
2. `read_chapter` - đọc lại văn bản trước đó để tìm lại âm điệu và nhịp điệu
3. `plan_chapter` - Hình dung mục tiêu, xung đột và cung bậc cảm xúc của chương này
4. `draft_chapter` — viết toàn bộ nội dung chương
5. `check_consistency` — Kiểm tra tính nhất quán với dữ liệu trạng thái (phải sau bản nháp)
6. `commit_chapter` - Gửi bản thảo cuối cùng, trả về các trường dữ kiện (`arc_end_reached`/`next_chapter`, v.v.), bước tiếp theo được điều khiển bởi Reminder

### Quy tắc di chuyển của tiểu bang

Hệ thống nội bộ chia trạng thái chạy thành hai lớp:

- **Giai đoạn** — Giai đoạn lớn, cho biết tác phẩm hiện đang ở giai đoạn chuẩn bị, giai đoạn viết hay đã hoàn thành
- **Dòng** — Quy trình hiện hoạt hiện tại, cho biết hệ thống đang ghi bình thường, đang xem xét, viết lại, đánh bóng hay đang xử lý sự can thiệp của người dùng vào lúc này

#### Phase

`Phase` áp dụng quy tắc "chỉ chuyển tiếp, không lùi":

```text
init -> premise -> outline -> writing -> complete
  \-------> outline ------^
  \--------------> writing
```

nghĩa:

- `init` - Tác vụ đã được tạo nhưng cấu hình ổn định chưa được hình thành
- `premise` — Đã lưu tiền đề câu chuyện
- `outline` — Đề cương đã được lưu và có thể nhập vào văn bản chính thức
- `writing` — đã bước vào giai đoạn tạo chương
- `complete` — Phần cuối của toàn bộ quá trình đặt sách

Mô tả quy tắc:

- Cho phép cập nhật đồng hình, chẳng hạn như `writing -> writing`
- Cho phép chuyển tiếp, ví dụ: `outline -> writing`
- Không được phép rollback như `writing -> premise`, `complete -> writing`

#### Flow

`Flow` chỉ mô tả các quy trình hoạt động trong thời gian viết, cho phép chuyển đổi giữa một số quy trình công việc:

```text
writing   -> reviewing / rewriting / polishing / steering / writing
reviewing -> writing / rewriting / polishing / steering / reviewing
rewriting -> writing / steering / rewriting
polishing -> writing / steering / polishing
steering  -> writing / reviewing / rewriting / polishing / steering
```

nghĩa:

- `writing` — Chuyển sang chương tiếp theo một cách bình thường
- `reviewing` — Biên tập viên đang xem xét
- `rewriting` — xử lý các chương phải viết lại
- `polishing` — xử lý các chương chỉ cần đánh bóng
- `steering` — Sự can thiệp của người dùng đang được đánh giá và xử lý

Mô tả quy tắc:

- Cho phép `writing -> reviewing`, chẳng hạn như kích hoạt đánh giá sau khi gửi chương
- Cho phép `reviewing -> rewriting/polishing/writing`, tùy thuộc vào kết quả đánh giá
- Cho phép `steering -> writing/reviewing/rewriting/polishing`, xác định theo phạm vi can thiệp
- Không được phép nhảy bất thường rõ ràng, chẳng hạn như `rewriting -> reviewing`

Các quy tắc này hiện bị hạn chế thống nhất bằng cách xác thực nhẹ trong mã, ngăn chặn việc khôi phục trạng thái hoặc chuyển sang các nhánh quy trình không hợp lý.

### Quy hoạch dài hạn

Kế hoạch truyền thống là lập kế hoạch cho tất cả các chương cùng một lúc. Khi có hơn 300 chương, dàn ý trống rỗng, nhịp điệu như vội vã theo lịch trình. Hệ thống này sử dụng **La bàn + Quy hoạch cuộn ngang** để mô phỏng quá trình sáng tạo thực sự của các tác giả bài viết trực tuyến:

```
lập kế hoạch ban đầu                     cuối cung                      cuối cuộn
┌────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│ Hướng cuối cùng (la bàn)    │    │ Editor đánh giá cấp độ vòng cung      │    │ Editor Đánh giá ở cấp độ giấy       │
│ Bắt đầu 2 Khối lượng, theo dõi theo yêu cầu   │    │ tóm tắt vòng cung + ảnh chụp nhanh nhân vật     │    │ Tóm tắt tập               │
│ KHÔNG.1Chương chi tiết Arc        │ →  │ Architect Mở rộng cung tiếp theo  │ →  │ Architect Tạo độc lập   │
│ Vai trò + thế giới quan        │    │ Writer tiếp tục viết      │    │ tập tiếp theo + Cập nhật la bàn    │
└────────────────────┘    └─────────────────────┘    └─────────────────────┘
```

- **La bàn** — Hướng cuối cùng + hoạt động dài hạn + ước tính quy mô, mỗi ranh giới cuộn được Kiến trúc sư cập nhật và hướng câu chuyện có thể phát triển khi sáng tạo
- **Tạo theo yêu cầu** — Sau khi viết tập hiện tại, Architect tự động tạo tập tiếp theo dựa trên nội dung đã được viết. Kế hoạch ban đầu là tạo 2 tập làm điểm bắt đầu và các tập tiếp theo sẽ được tạo theo yêu cầu.
- **Skeleton Arc** — mục tiêu duy nhất + số chương ước tính, các chương chi tiết sẽ được mở rộng khi đạt được
- **Sáng tạo dần dần** — Mỗi lần bạn phát triển, hãy tham khảo phần tóm tắt trước đó, ảnh chụp nhanh ký tự và quy tắc văn phong, và bạn càng viết sâu thì bạn sẽ càng chính xác hơn.
- **Mẫu nhịp điệu phổ quát** — Vòng cung đột phá tăng trưởng / Vòng cung đối đầu cạnh tranh / Vòng cung khám phá và khám phá / Vòng cung xung đột ác cảm / Vòng cung chuyển tiếp hàng ngày, mỗi loại vòng cung có mật độ tham chiếu và ánh xạ chủ đề phù hợp

### Quản lý bối cảnh dài

Cuốn tiểu thuyết hơn 500 chương sử dụng tóm tắt ba cấp độ + quy trình nén bốn cấp độ + khuyến nghị thông minh:

```
cuộn(Volume）→ Tóm tắt tập
└── cung(Arc）→ tóm tắt vòng cung + ảnh chụp nhanh nhân vật + quy tắc phong cách
    └── chương(Chapter）→ Tóm tắt chương (Cửa sổ trượt gần đây3chương)
```

- **Tóm tắt theo cấp bậc** — sử dụng tóm tắt chương cho khoảng cách gần, tóm tắt cung cho khoảng cách giữa và tóm tắt tập cho khoảng cách xa. Nén từng lớp sẽ không làm mất thông tin.
- **Khuyến nghị về các chương liên quan** — Khi viết mỗi chương, hãy xem lại các chương lịch sử từ bốn khía cạnh là điềm báo, sự xuất hiện của nhân vật, sự thay đổi trạng thái và các mối quan hệ. Người viết được khuyến khích đọc lại theo yêu cầu.
- **Xem trước chương tiếp theo** — Tải dàn ý chương tiếp theo và giúp Người viết thiết kế đoạn kết và lời báo trước ở cuối chương.
- **Phát hiện ranh giới vòng cung** — Tự động xác định phần cuối của cung/tập, kích hoạt xem xét, tạo tóm tắt và giải phóng cung/tập tiếp theo

#### Đường dẫn nén ngữ cảnh

Khi hội thoại vượt quá cửa sổ ngữ cảnh mô hình, nó sẽ được nén từng bước từ mức chi phí thấp đến chi phí cao:

```
ToolResultMicrocompact → LightTrim → StoreSummaryCompact → FullSummary
     Dọn dẹp kết quả công cụ cũ        Cắt ngắn văn bản dài      store không LLM nén      LLM Bản tóm tắt
```

- **StoreSummaryCompact** — Dành riêng cho Writer, thay thế trực tiếp các tin nhắn cũ bằng các tóm tắt chương hiện có, ảnh chụp nhanh nhân vật và tài khoản báo trước trong cửa hàng mà không tốn chi phí LLM
- **Tùy chỉnh tiểu thuyết tóm tắt đầy đủ** — Người viết sử dụng các từ gợi ý tóm tắt hướng tới tính liên tục của câu chuyện và yêu cầu rõ ràng việc giữ lại trạng thái nhân vật, manh mối báo trước, xem xét các mục cần sửa đổi và các điểm neo theo phong cách
- **Gói khôi phục nén** — Tự động đưa sơ đồ chương hiện tại, dàn ý và ảnh chụp nhanh ký tự sau FullSummary để ngăn Người viết khỏi "mất trí nhớ" sau khi nén
- **Fuse** — Tự động bỏ qua và hiển thị cảnh báo khi quá trình nén không thành công liên tục. Sử dụng chế độ nửa mở và tự động thử lại ở vòng tiếp theo.
- **Ước tính mã thông báo CJK** — `runes × 1.5` của Trung Quốc, không có độ trễ kích hoạt nén do đánh giá thấp `bytes/4`
- **Độ dốc sức khỏe TUI** — hiển thị thời gian thực màu xanh lá cây chiếm chỗ ngữ cảnh (<70%)→màu vàng(70-85%)→màu đỏ(>85%)

## Bắt đầu nhanh

```bash
# Cài đặt bằng một cú nhấp chuột (macOS/Linux, không cần Go)
curl -fsSL https://raw.githubusercontent.com/voocel/ainovel-cli/main/scripts/install.sh | sh

# Cài đặt phiên bản được chỉ định
curl -fsSL https://raw.githubusercontent.com/voocel/ainovel-cli/main/scripts/install.sh | sh -s -- v1.2.3

# Hoặc cài đặt qua Go
go install github.com/voocel/ainovel-cli/cmd/ainovel-cli@latest

# Xem phiên bản/cập nhật lên phiên bản mới nhất
ainovel-cli --version
ainovel-cli update

# Khi chạy lần đầu tiên, tự động nhập quá trình khởi động (chọn Nhà cung cấp → nhập Khóa API → URL cơ sở → tên model)
ainovel-cli
```

> Cài đặt Windows hoặc thủ công: Vào [Releases](https://github.com/voocel/ainovel-cli/releases/latest) để tải xuống gói dành cho nền tảng tương ứng.

### Docker

Hình ảnh Docker phù hợp để chạy các tác vụ dài không cần đầu trên máy chủ/NAS và bạn cũng có thể sử dụng `-it` để vào TUI. Nên gắn thư mục cấu hình và công việc vào máy chủ:

```bash
mkdir -p config workspace

# TUI
docker run --rm -it \
  -v "$PWD/config:/root/.ainovel" \
  -v "$PWD/workspace:/workspace" \
  ghcr.io/voocel/ainovel-cli:latest

# Headless
docker run --rm \
  -v "$PWD/config:/root/.ainovel" \
  -v "$PWD/workspace:/workspace" \
  ghcr.io/voocel/ainovel-cli:latest \
  --headless --prompt "Viết một cuốn tiểu thuyết viễn tưởng dài tập phương Đông, nhân vật chính bắt đầu từ một thị trấn nhỏ biên giới"
```

Bạn cũng có thể sử dụng tính năng Soạn thư:

```bash
docker compose run --rm ainovel
docker compose run --rm ainovel --headless --prompt "Viết một truyện ngắn hồi hộp"
```

Sau khi vào TUI, giai đoạn khởi động hỗ trợ hai tương tác trước:

- `bắt đầu nhanh`: Nhập trực tiếp sáng tạo trong 1 câu
- `Đồng sáng tạo quy hoạch`: Nhiều vòng đối thoại với AI để làm rõ yêu cầu, **dự thảo hướng dẫn sáng tạo được đồng bộ theo thời gian thực ở bên phải**; AI chủ động đưa ra 1-3 gợi ý hướng dẫn trong mỗi vòng, nhấn phím số để điền vào ô nhập và nhấn `Ctrl+S` để vào tạo chính thức

Cả hai chế độ cuối cùng sẽ hội tụ vào cùng một hướng dẫn sáng tạo và sau đó vào cùng một công cụ sáng tạo.

### Quản lý nhiều tiểu thuyết

Mỗi cuốn tiểu thuyết được liên kết với thư mục khởi động và sản phẩm nằm trong `{cwd}/output/novel/`. Bắt đầu bằng cách thay đổi thư mục = thay đổi một, bắt đầu lại `cd` = tự động khôi phục từ điểm kiểm tra mới nhất. Định cấu hình chia sẻ toàn cầu `~/.ainovel/config.json` mà không cần sao chép.

### Tệp cấu hình

Khi chạy lần đầu tiên, file cấu hình `~/.ainovel/config.json` được tạo tự động và có thể chỉnh sửa trực tiếp để điều chỉnh cài đặt sau này. Xóa tệp cấu hình và chạy lại sẽ vào lại quá trình khởi động.

Bạn cũng có thể tạo file cấu hình theo cách thủ công, tham khảo `~/.ainovel/config.example.jsonc` (được tạo tự động trong khi khởi động).

```jsonc
{
  "provider": "openrouter",
  "model": "google/gemini-2.5-flash",
  "providers": {
    "openrouter": {
      "api_key": "sk-or-v1-xxx",
      "base_url": "https://openrouter.ai/api/v1",
      "models": ["google/gemini-2.5-flash", "google/gemini-2.5-pro"],
      "extra": {
        "user_agent": "my-client/1.0",
        "headers": { "X-Custom-Client": "my-client" }
      }
    }
  },
  "style": "default"
}
```

#### Thứ tự tìm kiếm tệp cấu hình (cái sau ghi đè cái trước)

1. `~/.ainovel/config.json` - cấu hình toàn cầu
2. `./.ainovel/config.json` - Bảo hiểm cấp dự án (tùy chọn)
3. `--config path/to/config.json` — đặc tả dòng lệnh

> `.ainovel/` cấp dự án là hình ảnh phản chiếu của `~/.ainovel/` toàn cầu: có cùng cấu trúc, ngoại trừ thư mục gốc được thay đổi từ thư mục chính sang dự án hiện tại. Đặt `./.ainovel/config.json` để cấu hình và đặt `./.ainovel/rules/*.md` để viết quy tắc (xem phần "Xóa hương vị AI và quy tắc tùy chỉnh" bên dưới để biết chi tiết). Thư mục này chứa các khóa và được thêm vào `.gitignore` theo mặc định.

Mô tả các quy tắc bảo hiểm:

- Các trường vô hướng ghi đè lên trường trước bằng trường sau, chẳng hạn như `provider`, `model`, `style`
- `providers` và `roles` được hợp nhất theo khóa và các mục có cùng tên được ghi đè theo trường.
- Các trường không được điền sẽ kế thừa cấu hình cấp cao hơn. Ví dụ: nếu cấu hình cấp dự án chỉ ghi `base_url` thì `api_key` trong cấu hình chung sẽ được giữ lại.
- Hiện tại, không hỗ trợ sử dụng chuỗi trống để xóa rõ ràng các giá trị hiện có ở lớp trên; nếu bạn cần xóa chúng, vui lòng chỉnh sửa trực tiếp tệp cấu hình có mức ưu tiên cao hơn.

> ⚠️ Giá trị của `provider` (và `roles.*.provider`) là **tên khóa** trong `providers` - một con trỏ, không phải tên giao thức. Nếu `provider` được chuyển sang tài khoản không tồn tại trong `providers` toàn cầu ở cấp dự án thì thông tin xác thực của tài khoản (`api_key`/`base_url`) phải được cung cấp đồng thời ở cấp dự án, nếu không thì "Không có thông tin xác thực nào được định cấu hình" sẽ được báo cáo khi khởi động.

`providers.<name>.models` là trường tùy chọn, dùng để khai báo danh sách các model thuộc nhà cung cấp này được phép chuyển đổi trong bảng TUI `/model`; nếu không được định cấu hình, hệ thống sẽ quay trở lại mô hình nhà cung cấp đã xuất hiện trong tệp cấu hình hiện tại.

`providers.<name>.extra` là cấu hình cấp nhà cung cấp sẽ được chuyển đến máy khách HTTP cơ bản. Nó phù hợp để định cấu hình các trường nhận dạng proxy như `user_agent`, `headers` và `anthropic_beta`. `providers.<name>.extra_body` là tham số mở rộng nội dung yêu cầu. Đừng trộn lẫn cả hai.

## Báo cáo chẩn đoán

Nhập `/diag` vào TUI có thể thực hiện phân tích chẩn đoán trên sản phẩm đầu ra của tiểu thuyết hiện tại và đưa ra các phát hiện có thể thực thi cũng như đề xuất cải tiến.

Chẩn đoán bao gồm bốn khía cạnh:

- **Quy trình** — Viết lại độ trễ chu kỳ, hướng dẫn lái chưa được sử dụng, trạng thái quy trình/giai đoạn bất thường và bỏ qua chương
- **Chất lượng** — Điểm thấp liên tục trong các khía cạnh đánh giá, tỷ lệ hoàn thành hợp đồng, tỷ lệ viết lại và số lượng chương bất thường.
- **Quy hoạch** — Điềm báo trì trệ, la bàn lỗi thời, dàn ý cạn kiệt, thiếu phần tóm tắt
- **Bối cảnh** — thiếu ký tự, khoảng trống về dòng thời gian, dữ liệu mối quan hệ trì trệ

Mỗi phát hiện bao gồm: mô tả vấn đề, bằng chứng dữ liệu và đề xuất cải tiến (chỉ vào lời nhắc/luồng/cấu hình cụ thể).

`/diag` cũng sẽ viết **giải mẫn cảm** `meta/diag-export.md` (xóa văn bản chính của tiểu thuyết và chỉ giữ lại bộ khung hành vi như lệnh gọi công cụ, chuỗi lỗi, thời gian lặp lại, v.v.). Nếu bạn gặp phải sự cố vòng lặp/gián đoạn vô hạn, chỉ cần đăng nó lên vấn đề GitHub để người bảo trì có thể xác định vị trí của nó nếu không thể lấy được dữ liệu cục bộ.

## Chân dung giả

Đặt bài viết tham khảo vào thư mục `simulate/` của thư mục khởi động hiện tại, sau đó nhập `/simulate` vào TUI. Hệ thống sẽ đọc đệ quy các tệp `.txt`, `.md` và `.markdown`, sử dụng mô hình kiến ​​trúc để phân tích kho văn bản và viết:

```text
output/novel/meta/simulation_profile.json
```

Khi `/simulate` được chạy lại, các tệp không thay đổi sẽ bị bỏ qua theo `relative_path + sha256`; nếu không có nội dung mới hoặc thay đổi, "Chân dung là mới nhất" sẽ được nhắc và LLM sẽ không được gọi. Nếu đã có hình ảnh và xuất hiện bài viết mới hoặc sửa đổi trong `simulate/`, hệ thống sẽ tiếp tục tổng hợp dựa trên hình ảnh gốc.

Bạn cũng có thể nhập các ảnh chân dung đã tạo trước đó để tránh phải phân tích lặp lại cùng một loạt bài viết:

```text
/simulate
/importsim ./profile.json
```

`/importsim` chỉ chấp nhận `simulation_profile.v1` JSON được tạo bởi hàm này và hợp nhất nó theo dấu vân tay kho văn bản. Các nguồn trùng lặp sẽ bị bỏ qua. Chỉ các tệp dọc từ các nguồn đáng tin cậy mới được nhập; nội dung đã nhập sẽ trở thành tài liệu tham khảo theo ngữ cảnh cho các Đại lý tiếp theo. Bức chân dung sẽ được đưa vào `novel_context` ở dạng nhỏ gọn, điều phối viên, Kiến trúc sư, Nhà văn và Biên tập viên có thể đọc được; mỗi Tác nhân chỉ dựa trên cấu trúc, nhịp điệu, câu móc và kỹ thuật để thu hút người đọc và không sao chép cách diễn đạt hoặc cài đặt độc quyền ban đầu.

## Nhập khẩu

Bằng cách nhập `/import <đường dẫn tập tin>` trong TUI, một cuốn tiểu thuyết hiện có có thể được nhập ngược lại: trước tiên hãy chia nó thành các chương, sau đó sử dụng LLM để suy ra tiền đề/nhân vật/thế giới quan/sơ đồ phân cấp/la bàn, sau đó tải xuống từng chương. Văn bản gốc được hoàn thành dưới dạng tập đầu tiên và có thể được tiếp tục trong bộ truyện. Sau khi quá trình nhập hoàn tất, quá trình nhập sẽ tự động được tiếp tục và tiếp tục - điều phối viên sẽ đánh giá/tóm tắt ở cuối tập đầu tiên, thêm tập mới và tiếp tục từ chương tiếp theo.

```
/import ~/tiểu thuyết của tôi.txt              # Nhập từ đầu và đẩy lùi foundation
/import ~/tiểu thuyết của tôi.txt from=50      # Từ lần đầu tiên 50 Nhập từng chương (bỏ qua đẩy ngược)
```

**Quy tắc chia chương**: Tự động nhận dạng các định dạng tiêu đề này (đầu dòng, có thể bắt đầu bằng `#`/`##` Markdown, gói `【】`/`〖〗`, khoảng trắng toàn chiều rộng, tương thích với mã hóa GBK/BOM):

- Số tiếng Trung: `Chương 1` `KHÔNG.3trở lại` `Chương 10` `Tập 2` `Phần 5` `màn 2`, `Tập 1` độc lập, số hỗ trợ viết hoa (`Chương 1`) và có thể có phụ đề (`Chương 3: Trận chiến quyết định`)
- Đơn vị đặc biệt Trung Quốc: `Lời mở đầu` `cái nêm` `Giới thiệu` `Lời nói đầu` `kết thúc` `Chương cuối cùng` `phần tái bút` `thêm` `Gaiden`
- Tiếng Anh: `Chapter 1` `Chapter II`, `Prologue` `Epilogue`, có phụ đề (`Chapter 1: The Beginning`)

Nếu nó nhắc **"Không nhận dạng chương nào"**, vui lòng xác nhận rằng tệp này thực sự là một văn bản tiểu thuyết dựa trên chương (tiêu đề chương nằm trên dòng riêng và nằm ở đầu dòng).

> Việc nhập là phát lại xác định và không thông qua Điều phối viên; văn bản gốc sẽ được ghi lại nguyên văn các chương hoàn chỉnh nên phù hợp để “tiếp theo cùng một cuốn sách”. Nếu bạn chỉ muốn sử dụng cài đặt để tạo tác phẩm mới, vui lòng sử dụng phương pháp thông thường để mở sách mới và mô tả cài đặt kiểu mong muốn trong yêu cầu.

## Xuất khẩu

Nhập `/export` vào TUI để hợp nhất và xuất các chương đã hoàn thành. Mặc định là TXT và được ghi vào `{novelDir}/{NovelName}.txt`. Xuất là thao tác chỉ đọc và bạn có thể nhận được "sản phẩm hoàn thiện hiện tại" bất kỳ lúc nào trong quá trình viết mà không ảnh hưởng đến thao tác của Điều phối viên.

Định dạng được xác định bởi hậu tố đường dẫn đầu ra (`.txt`/`.epub`):

```text
/export                            # mặc định TXT，{novelDir}/{NovelName}.txt
/export ~/điểm sáng.txt                  # hậu tố .txt → TXT
/export ~/điểm sáng.epub                 # hậu tố .epub → EPUB（Apple Books / đọc WeChat / Kindle bộ chuyển đổi có thể đọc được)
/export from=10 to=30 --overwrite  # Khoảng thời gian của chương + che phủ
/export from=10 ~/x.epub --overwrite
```

- **TXT** — `"Tên sách"` → Tách tập → Văn bản chương (chế độ xếp lớp dài sẽ tự động thêm phần tách tập). Hai loại dữ liệu nội bộ **không được nhập hoặc xuất**: tiền đề (bản thiết kế sáng tạo, bao gồm thông tin cơ bản như người đọc mục tiêu/khu vực hạn chế viết, được viết cho tác giả và công cụ) và tách vòng cung (cung là một cấu trúc nội bộ chi tiết theo quan điểm của người đọc). Nhà xuất khẩu tạo ra "Tiêu đề Chương N" một cách thống nhất và các tiêu đề trùng lặp (`# KHÔNG.Nchương…` hoặc `# Tên chương`) đi kèm với người viết trong văn bản sẽ bị loại bỏ.
- **EPUB** — Vùng chứa tiêu chuẩn EPUB 3 có trang bìa, mục lục, XHTML phân chia theo chương và số nhận dạng được lấy ổn định dựa trên nội dung (tái xuất cùng một cuốn sách sẽ được người đọc nhận ra là phiên bản cập nhật). Không có ảnh bìa.

Các chương chưa hoàn thành trong phạm vi sẽ bị bỏ qua và hiển thị trong kết quả và không bị coi là lỗi.

#### Sử dụng các mô hình khác nhau theo vai trò

Chỉ định các mô hình khác nhau cho các tác nhân khác nhau thông qua trường `roles` và các vai trò chưa được định cấu hình sẽ sử dụng mô hình mặc định:

```jsonc
{
  "provider": "openrouter",
  "model": "google/gemini-2.5-flash",
  "providers": {
    "openrouter": { "api_key": "sk-or-v1-xxx", "base_url": "https://openrouter.ai/api/v1" },
    "anthropic": { "api_key": "sk-ant-xxx" }
  },
  "roles": {
    "writer": { "provider": "anthropic", "model": "claude-sonnet-4" },
    "architect": { "provider": "openrouter", "model": "google/gemini-2.5-pro" }
  }
}
```

Các vai trò có thể định cấu hình: `coordinator`/`architect`/`writer`/`editor`

#### Proxy tùy chỉnh

Chọn bất kỳ Nhà cung cấp nào và điền địa chỉ proxy hoặc sử dụng Proxy tùy chỉnh và chỉ định loại giao thức API. `api_key` của proxy tùy chỉnh là tùy chọn; nếu proxy của bạn không yêu cầu xác thực, bạn có thể bỏ qua nó:

```jsonc
{
  "provider": "my-proxy",
  "model": "gpt-4o",
  "providers": {
    "my-proxy": {
      "type": "openai",
      "base_url": "https://proxy.example.com/v1",
      "extra": {
        "user_agent": "my-client/1.0",
        "headers": { "X-Custom-Client": "my-client" }
      }
    }
  }
}
```

Nhà cung cấp được hỗ trợ: `openrouter`/`anthropic`/`gemini`/`openai`/`deepseek`/`qwen`/`glm`/`grok`/`ollama`/`bedrock` và bất kỳ proxy tùy chỉnh nào.

Nếu proxy là giao thức Anthropic và yêu cầu các trường nhận dạng máy khách thì `type` phải được đặt thành `anthropic`, `anthropic_beta` phải được đặt trên `extra` và các tiêu đề HTTP như Không gỉ phải được đặt trong `extra.headers`:

```jsonc
{
  "provider": "claude-proxy",
  "model": "claude-sonnet-4-6",
  "providers": {
    "claude-proxy": {
      "type": "anthropic",
      "api_key": "sk-xxx",
      "base_url": "https://proxy.example.com",
      "extra": {
        "user_agent": "claude-code/2.1.183",
        "anthropic_beta": "claude-code-20250219",
        "headers": {
          "X-Stainless-Lang": "js",
          "X-Stainless-Package-Version": "0.94.0",
          "X-Stainless-Runtime": "node"
        }
      }
    }
  }
}
```

Giới thiệu về `api_key`:

- `openrouter`/`anthropic`/`gemini`/`openai`/`deepseek`/`qwen`/`glm`/`grok` Loại giao diện hosting này thường cần điền `api_key`
- `ollama` và `bedrock` được phép để trống `api_key`; Bedrock cần định cấu hình `region`, `access_key_id` và `secret_access_key` trong `extra` (`session_token` là tùy chọn)
- Các tác nhân tùy chỉnh chỉ định rõ ràng `type` được phép để trống `api_key`.

Ví dụ: cấu hình `ollama` cục bộ:

```jsonc
{
  "provider": "ollama",
  "model": "qwen3:latest",
  "providers": {
    "ollama": {
      "base_url": "http://localhost:11434/v1"
    }
  }
}
```

### Phong cách viết

Chuyển qua trường `style` của tệp cấu hình:

- `default` — Phong cách phổ quát
- `suspense` — lý luận hồi hộp
- `fantasy` — Truyện cổ tích giả tưởng
- `romance` — Lãng mạn

### Xóa hương vị AI và các quy tắc tùy chỉnh

Có một đường cơ sở phù hợp với AI được tích hợp sẵn (theo `assets/`, mặc định của nhà máy): danh sách đen cơ học `rules/default.md` (các câu thông thường/từ gây mệt mỏi, kiểm tra độ chắc chắn khi cam kết) + tiêu chí ngữ nghĩa `references/anti-ai-tone.md` (tránh người viết/biên tập viên chèn và bằng chứng).

Nếu bạn muốn phủ các tùy chọn của riêng mình, bạn không cần thay đổi mã nguồn: Trong thư mục `~/.ainovel/rules/` (toàn cầu, đặt bất kỳ `.md` nào, hợp nhất theo thứ tự từ điển của tên tệp) hoặc thư mục `./.ainovel/rules/` (đối với cuốn sách này, cũng đặt bất kỳ `.md` nào, hình dạng giống như toàn cầu), chỉ cần viết tùy chọn của bạn bằng tiếng bản địa** (chẳng hạn như "Đừng viết nhân vật chính là Madonna" "Sử dụng nhận thức cơ thể nhiều hơn") và người biên tập sẽ xem xét nó dựa trên ngữ nghĩa - định dạng không, không YAML. Nếu bạn muốn kiểm tra kỹ như "số từ/từ bị cấm", thì tùy ý thêm vấn đề phía trước ở đầu tài liệu. Phạm vi bao phủ và lớp phủ lân cận với đường cơ sở tích hợp có hiệu lực; xem [`rules.md.example`](rules.md.example) để biết các trường hoàn chỉnh.

## Cấu trúc đầu ra

Tất cả dữ liệu sáng tạo (chương, dàn ý, ký tự, tiến trình, v.v.) đều được lưu trong thư mục đầu ra. Khởi động lại sau khi bị gián đoạn sẽ tự động tiếp tục ghi từ tiến trình cuối cùng. Xóa thư mục đầu ra sẽ khởi động lại quá trình tạo.

```
output/{novel_name}/
├── chapters/           # bản thảo cuối cùng (Markdown）
│   ├── 01.md
│   └── ...
├── summaries/          # Tóm tắt chương (JSON）
├── drafts/             # Bản thảo chương
├── reviews/            # Báo cáo đánh giá
├── meta/
│   ├── premise.md      # Tiền đề câu chuyện
│   ├── outline.json    # Đề cương chương phẳng (chỉ các chương mở rộng)
│   ├── layered_outline.json # Đề cương phân cấp (Tập hiện tại + Khối lượng xem trước, chế độ độ dài tính năng)
│   ├── compass.json   # La bàn hướng kết thúc (Chế độ truyện dài)
│   ├── characters.json # hồ sơ nhân vật
│   ├── world_rules.json# quy tắc thế giới
│   ├── progress.json   # trạng thái tiến độ
│   ├── timeline.json   # dòng thời gian
│   ├── foreshadow.json # Tài khoản báo trước
│   ├── state_changes.json # Bản ghi thay đổi trạng thái nhân vật
│   ├── style_rules.json# Quy tắc kiểu viết (được tinh chỉnh khi tạo ranh giới cung)
│   ├── snapshots/      # Ảnh chụp nhanh trạng thái ký tự (dạng dài)
│   ├── checkpoints.jsonl # Step lớp học checkpoint(Bổ sung sau khi mỗi công cụ thành công)
│   ├── characters.md   # Hồ sơ nhân vật (phiên bản có thể đọc được)
│   └── world_rules.md  # Quy tắc thế giới (phiên bản có thể đọc được)
```

## Phục hồi điểm dừng

Viết một cuốn tiểu thuyết có thể mất nhiều giờ, thậm chí nhiều ngày. Sự cố, mất kết nối và Ctrl+C là những tình huống phổ biến. Hệ thống tự động phục hồi khi chạy lại cùng thư mục đó, không cần thao tác thủ công.

### Cảnh phục hồi

| Thời gian gián đoạn | Hành vi phục hồi |
|---|---|
| Giai đoạn lập kế hoạch (xây dựng thế giới quan/phác thảo) | Kiểm tra cài đặt đã lưu và tự động hoàn thành các mục còn thiếu |
| Một chương nào đó đang được viết (có bản thảo chưa được gửi) | Tiếp tục viết từ chương này, đọc bản thảo hiện có và tiếp tục |
| Đang trong quá trình xem xét | Đánh giá trình chỉnh sửa Retrigger |
| Hàng đợi viết lại/đánh bóng không bị xóa | Tiếp tục xử lý các chương cần viết lại |
| Việc mở rộng cung/tập bị gián đoạn (việc xem xét đã hoàn tất nhưng cung tiếp theo không được mở rộng) | Tự động phát hiện các cung/khối khung và kích hoạt mở rộng Kiến trúc sư |
| Sự can thiệp của người dùng chưa hoàn thành | Tiêm lại lệnh can thiệp cuối cùng |
| Nghỉ viết bình thường | Tiếp tục từ chương tiếp theo |

### Nguyên tắc làm việc

Tất cả các sản phẩm tạo đều được lưu giữ trong thư mục `output/`. Sau khi mỗi công cụ thực thi thành công, điểm kiểm tra (`meta/checkpoints.jsonl`) sẽ được ghi. Khi khởi động lại:

1. Đọc `progress.json` + điểm kiểm tra gần đây + tín hiệu đang chờ xử lý
2. Tạo hướng dẫn khôi phục chính xác đến từng bước (chẳng hạn như "Bản nháp Chương 7 đã được đặt, vui lòng tiếp tục check_consistency")
3. Khi `Prompt` khởi động Điều phối viên, hãy nhập một vòng lặp dài và tiếp tục tạo.

> Ghi tệp sử dụng các thao tác nguyên tử temp + fsync + đổi tên và dữ liệu hiện có sẽ không bị hỏng ngay cả khi mất điện trong quá trình ghi.

## Can thiệp theo thời gian thực (Steer)

Trong quá trình tạo, bạn có thể đưa ra nhận xét sửa đổi bất kỳ lúc nào thông qua hộp nhập liệu, **không cần tạm dừng hoặc khởi động lại**.

### Chế độ TUI

Sau khi bắt đầu tạo, hộp nhập liệu phía dưới sẽ tự động chuyển sang chế độ can thiệp:

```
❯ Nâng dòng cảm xúc lên dòng thứ ba4Chương, tăng sự cạnh tranh giữa nhân vật nam và nữ chính
```

Nhấn Enter sau khi nhập, hệ thống sẽ tự động:
1. Ghi lệnh can thiệp vào `run.json` (để khắc phục sự cố)
2. Tiêm vào Điều phối viên đang chạy
3. Điều phối viên đánh giá phạm vi ảnh hưởng và quyết định xem có sửa đổi cài đặt, viết lại các chương hiện có hay điều chỉnh trong các chương tiếp theo hay không

### Ví dụ can thiệp

| Hướng dẫn can thiệp | Những phản hồi có thể có của hệ thống |
|---|---|
| "Đổi nhân vật chính thành nữ" | Sửa đổi cài đặt ký tự và đánh giá xem các chương đã viết có cần viết lại hay không |
| “Chuyển tiếp dòng cảm xúc đến Chương 4” | Điều chỉnh lại dàn ý, có thể viết lại Chương 4 trở đi |
| "Thêm nhân vật phản diện" | Cập nhật profile nhân vật và quy luật thế giới sẽ được giới thiệu ở các chương tiếp theo |
| "Tốc độ quá chậm, tăng tốc độ" | Điều chỉnh mật độ phác thảo của các chương tiếp theo |

## Ý tưởng thiết kế

> **Chuyển độ phức tạp từ mã sang mô hình. ** Càng ít mã thì càng ít lỗi xảy ra. Giao quyền ra quyết định cho nhân vật có khả năng đưa ra quyết định tốt hơn.

### Driver LLM càng đơn giản càng ổn định

- **Quyền ra quyết định thuộc về LLM** — Tất cả các quyết định trong quy trình đều do Điều phối viên đưa ra độc lập và Người tổ chức không can thiệp. Lỗi cấu trúc được trả về khi công cụ bị lỗi và LLM có quyền thử lại hoặc điều chỉnh chiến lược theo ý mình
- **Công cụ chỉ trả về dữ kiện** — Atomic IO + ghi điểm kiểm tra, giá trị trả về là trường dữ kiện JSON (`final_verdict`/`pending_rewrites`/`arc_end_reached`), không có bất kỳ chuỗi lệnh nào
- **Lời nhắc điều khiển mỗi vòng** - Máy chủ đọc lớp thực tế trước mỗi vòng gọi LLM, chạy trình tạo hàm thuần túy để tạo nội dung `<system-reminder>`, hướng dẫn không nhập lịch sử liên tục và được tính toán lại từ dữ kiện trong mỗi vòng
- **Kiểm soát cổng vật lý StopGuard** — Tại `Phase ≠ Complete`, Điều phối viên không khả dụng về mặt vật lý đối với `end_turn` và quá trình nâng cấp sẽ chỉ chấm dứt sau khi việc chặn liên tục vượt quá giới hạn.
- **Từ chối sự điều phối phức tạp** — không có hàng đợi nhiệm vụ, không có bộ lập lịch, không có công cụ chính sách. Chạy điều phối viên là luồng điều khiển duy nhất
- **Mô hình càng mạnh thì lợi ích càng lớn** — Kiến trúc trao quyền ra quyết định trong ngữ nghĩa lời nhắc và công cụ. Sau khi nâng cấp mô hình sẽ được hưởng lợi trực tiếp và không cần thay đổi dòng Host.

### Vòng khép kín hoàn toàn tự động

Nhập một câu và xuất ra cuốn tiểu thuyết hoàn chỉnh:

```
"Viết một cuốn tiểu thuyết hồi hộp" → Xây dựng thế giới quan → vai trò thiết kế → Đề cương quy hoạch
                → viết từng chương → đánh giá chất lượng → tự động viết lại
                → tóm tắt cấp độ cung → ảnh chụp nhanh nhân vật → cuốn sách hoàn chỉnh
```

- **Điều phối viên tự lập kế hoạch** — đọc lớp thực tế trong một vòng lặp dài + Lời nhắc quyết định bước tiếp theo mà không cần sự can thiệp của Máy chủ
- **Sáng tạo độc lập của người viết** — Mỗi chương hoàn thành một cách độc lập vòng lặp khép kín hoàn chỉnh của kế hoạch → bản nháp → kiểm tra → cam kết
- **Đánh giá độc lập của biên tập viên** — Phân tích các vấn đề về cấu trúc giữa các chương, phán quyết đầu ra và phạm vi tác động
- **Kiến trúc sư tự xây dựng** - rút ra các cài đặt hoàn chỉnh từ một câu yêu cầu và tự động thực hiện quy hoạch tiếp theo khi đạt đến ranh giới cung/khối lượng
- **Quản lý báo trước tự động** — Toàn bộ quá trình trồng, tiến và tái chế được chính Đại lý theo dõi
- **Điều khiển nhịp điệu tự động** — Theo dõi các dòng tường thuật và lịch sử loại câu nối để tránh các cấu trúc có cấu trúc tương tự trong các chương liên tiếp

### Tách rời sự kiện và hướng dẫn

Công cụ chỉ trả về dữ kiện và các hướng dẫn được Nhắc nhở tính toán lại ở cấp độ dữ kiện mỗi vòng:

- `commit_chapter`/`save_review` trả về các sự kiện có cấu trúc (`final_verdict`/`pending_rewrites`/`arc_end_reached`/`next_chapter`) mà không kèm theo bất kỳ chuỗi `[hệ thống]` nào
- Trình tạo hàm thuần túy trong `internal/host/reminder/` đọc `Progress` + `Outline` và tạo `<system-reminder>` trong mỗi vòng trước lượt: `flow` (làm gì bây giờ/hãm cuối cung)/`queue_guard` (không có chương mới cho đến khi hàng đợi được xóa)/`book_complete` (chỉ phát hành sau khi đọc hết toàn bộ cuốn sách). Vỏ vật lý do `StopGuard` chịu nhưng `end_turn` từ chối làm như vậy khi `phase≠Complete`
- Nhắc nhở chỉ tồn tại một vòng, không đi vào lịch sử và không tham gia nén; các quy tắc có các bài kiểm tra đơn vị và sự xuống cấp có thể được ghi lại bằng hồi quy

Bằng cách này, các hướng dẫn sẽ không bị nuốt chửng bởi các cuộc gọi dây chuyền cũng như không bị trôi trong các sản phẩm công cụ. Để sửa lỗi, bạn chỉ cần thêm trình tạo + bài kiểm tra.

## ngăn xếp công nghệ

- **Go 1.25** — Ngôn ngữ chính
- **[agentcore](https://github.com/voocel/agentcore)** — kernel Agent tối giản (gọi công cụ + phát trực tuyến)
- **[litellm](https://github.com/voocel/litellm)** — Thích ứng giao diện LLM hợp nhất
- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** — khung TUI đầu cuối

## License

MIT

Dự án này tích cực tham gia và công nhận [linux.do Cộng đồng](https://linux.do/).
