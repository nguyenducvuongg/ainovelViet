# ainovelViet

> **Công cụ viết tiểu thuyết AI tự động hoàn toàn bằng tiếng Việt.**
> Từ một câu yêu cầu → một cuốn tiểu thuyết hoàn chỉnh, không cần can thiệp thủ công.

Điều phối viên điều khiển ba tác nhân phụ (Kiến trúc sư / Nhà văn / Biên tập viên) trong một vòng lặp dài, hoàn thành toàn bộ quá trình lên kế hoạch → viết → đánh giá → tóm tắt chỉ trong **một lần chạy**.

## ✨ Tính năng nổi bật

- **Cộng tác đa tác nhân** — Điều phối viên lên lịch cho ba tác nhân phụ (Kiến trúc sư / Nhà văn / Biên tập viên) đưa ra quyết định độc lập trong quá trình sáng tạo
- **Vòng lặp dài do LLM điều khiển** — Viết toàn bộ cuốn sách trong một lời nhắc; Máy chủ không can thiệp vào lịch trình. Thiết kế đơn giản, ổn định cao
- **Khôi phục điểm dừng chính xác** — Ghi điểm kiểm tra sau mỗi lần công cụ thực thi thành công, có thể khôi phục chính xác đến từng bước lập kế hoạch / viết / kiểm tra / cam kết sau sự cố
- **Lập kế hoạch cuộn hai lớp** — Truyện dài không lên kế hoạch tất cả chương cùng lúc; ban đầu chỉ lên khung 2 phần đầu + chi tiết phần thứ nhất; Kiến trúc sư tự động mở rộng khi viết đến ranh giới cung
- **Gợi ý chương liên quan thông minh** — Khi viết mỗi chương, tự động gợi ý các chương lịch sử liên quan từ 4 góc độ: báo trước, ngoại hình nhân vật, thay đổi trạng thái và mối quan hệ
- **Chiến lược ngữ cảnh thích ứng** — Tự động chuyển giữa cửa sổ đầy đủ / trượt / tóm tắt phân cấp dựa trên số chương; hỗ trợ trên 500 chương
- **Đánh giá chất lượng 7 chiều** — Tính nhất quán thiết lập, hành vi nhân vật, nhịp điệu, mạch lạc câu chuyện, báo trước, điểm hấp dẫn và chất lượng thẩm mỹ (miêu tả / kể chuyện / đối thoại / ngôn từ / cảm xúc)
- **Can thiệp theo thời gian thực** — Nhập nhận xét sửa đổi bất kỳ lúc nào mà không cần dừng máy; hệ thống tự đánh giá phạm vi ảnh hưởng và viết lại
- **Giao diện TUI tích hợp** — Quan sát tiến trình thời gian thực, can thiệp và bắt đầu trực tiếp từ yêu cầu
- **Hỗ trợ nhiều LLM** — OpenRouter / Anthropic / Gemini / OpenAI / DeepSeek / Ollama và nhiều hơn

---

## 🏛️ Kiến trúc hệ thống

Thiết kế cốt lõi: **LLM điều khiển, Máy chủ mỏng**. Điều phối viên tự xác định toàn bộ quy trình tạo sách trong một lần chạy; Máy chủ chỉ khởi động, khôi phục và quan sát sự kiện.

```
┌─────────────────────────────────────────────────┐
│               Host (Máy chủ mỏng)               │
│     khởi động / khôi phục / quan sát / can thiệp │
└──────────────────────┬──────────────────────────┘
                       │ một lần Prompt
┌──────────────────────▼──────────────────────────┐
│           Coordinator (LLM vòng lặp dài)         │
│  đọc novel_context → quyết định → gọi công cụ   │
└────┬──────────┬──────────┬──────────────────────┘
     │          │          │
 ┌───▼────┐ ┌───▼───┐ ┌────▼────┐
 │Architect│ │Writer │ │ Editor  │
 └───┬────┘ └───┬───┘ └────┬────┘
     └──────────┼──────────┘
                │ gọi công cụ (IO + checkpoint)
┌───────────────▼─────────────────────────────────┐
│                     Store                        │
│  Progress / Checkpoint / Outline / Drafts / ...  │
└─────────────────────────────────────────────────┘
```

| Thành phần | Vai trò |
|---|---|
| **Máy chủ (Host)** | Khởi động Điều phối viên, khắc phục sự cố, phát sự kiện tới TUI. Không ra quyết định kế hoạch |
| **Điều phối viên** | Người ra quyết định duy nhất; điều khiển toàn bộ: lên kế hoạch → viết → đánh giá → tóm tắt |
| **Tác nhân phụ** | Kiến trúc sư / Nhà văn / Biên tập viên — ngữ cảnh độc lập, cộng tác qua Store |
| **Công cụ** | IO nguyên tử + ghi điểm kiểm tra; chỉ trả về JSON dữ kiện, không có chuỗi hướng dẫn |

### Trách nhiệm tác nhân

| Tác nhân | Trách nhiệm | Công cụ |
|---|---|---|
| **Điều phối viên** | Lập kế hoạch tổng thể, xử lý đánh giá và can thiệp người dùng | `subagent` `novel_context` |
| **Kiến trúc sư** | Tạo tiền đề, dàn ý, hồ sơ nhân vật, quy luật thế giới | `novel_context` `save_foundation` |
| **Nhà văn** | Lên ý tưởng, viết, tự nhận xét và nộp chương độc lập | `novel_context` `read_chapter` `plan_chapter` `draft_chapter` `check_consistency` `commit_chapter` |
| **Biên tập viên** | Đọc văn bản gốc và đánh giá từ cấp độ cấu trúc và thẩm mỹ | `novel_context` `read_chapter` `save_review` `save_arc_summary` `save_volume_summary` |

### Quy trình viết

```
Yêu cầu → Architect lên khung + Cung đầu → Writer viết từng chương → Editor đánh giá cung
                                      ↑                   │
                                      ├── viết lại/đánh bóng ◄──┘
                                      │
                               Architect mở rộng cung/tập tiếp theo
                              (tham khảo tóm tắt trước + trạng thái nhân vật)
```

Nhà văn hoàn thành mỗi chương theo thứ tự cố định:

1. `novel_context` — Tải ngữ cảnh (tóm tắt trước, báo trước, trạng thái nhân vật, quy tắc phong cách)
2. `read_chapter` — Đọc lại văn bản trước để tìm lại âm điệu và nhịp điệu
3. `plan_chapter` — Hình dung mục tiêu, xung đột và cung bậc cảm xúc của chương
4. `draft_chapter` — Viết toàn bộ nội dung chương
5. `check_consistency` — Kiểm tra tính nhất quán với dữ liệu trạng thái (phải sau bản nháp)
6. `commit_chapter` — Nộp bản thảo cuối; trả về trường dữ kiện `arc_end_reached`/`next_chapter`

---

## 🚀 Bắt đầu nhanh

### Cài đặt

```bash
# Cài đặt một lệnh (macOS/Linux, không cần Go)
curl -fsSL https://raw.githubusercontent.com/nguyenducvuongg/ainovelViet/main/scripts/install.sh | sh

# Cài đặt phiên bản cụ thể
curl -fsSL https://raw.githubusercontent.com/nguyenducvuongg/ainovelViet/main/scripts/install.sh | sh -s -- v1.2.3

# Hoặc cài đặt qua Go
go install github.com/nguyenducvuongg/ainovelViet/cmd/ainovel-cli@latest

# Kiểm tra phiên bản / cập nhật lên phiên bản mới nhất
ainovel-cli --version
ainovel-cli update
```

> **Windows hoặc cài đặt thủ công:** Vào [Releases](https://github.com/nguyenducvuongg/ainovelViet/releases/latest) để tải gói phù hợp với nền tảng.

### Chạy lần đầu

```bash
# Chạy và làm theo hướng dẫn khởi động (chọn Provider → nhập API Key → URL → tên model)
ainovel-cli
```

Khi vào TUI, giai đoạn khởi động hỗ trợ hai chế độ:

- **Bắt đầu nhanh** — Nhập trực tiếp yêu cầu sáng tạo trong 1 câu
- **Đồng sáng tạo** — Nhiều vòng đối thoại với AI để làm rõ yêu cầu; bản thảo hướng dẫn được cập nhật thời gian thực ở bên phải; nhấn `Ctrl+S` để bắt đầu viết

### Docker

```bash
mkdir -p config workspace

# Chế độ TUI
docker run --rm -it \
  -v "$PWD/config:/root/.ainovel" \
  -v "$PWD/workspace:/workspace" \
  ghcr.io/nguyenducvuongg/ainovelViet:latest

# Chế độ Headless (không giao diện)
docker run --rm \
  -v "$PWD/config:/root/.ainovel" \
  -v "$PWD/workspace:/workspace" \
  ghcr.io/nguyenducvuongg/ainovelViet:latest \
  --headless --prompt "Viết một cuốn tiểu thuyết viễn tưởng phương Đông, nhân vật chính xuất phát từ thị trấn biên giới nhỏ"
```

Hoặc dùng Docker Compose:

```bash
docker compose run --rm ainovel
docker compose run --rm ainovel --headless --prompt "Viết một truyện ngắn hồi hộp"
```

---

## ⚙️ Cấu hình

### Tệp cấu hình

Khi chạy lần đầu, file `~/.ainovel/config.json` được tạo tự động. Bạn cũng có thể tạo thủ công, tham khảo `~/.ainovel/config.example.jsonc`.

```jsonc
{
  "provider": "openrouter",
  "model": "google/gemini-2.5-flash",
  "providers": {
    "openrouter": {
      "api_key": "sk-or-v1-xxx",
      "base_url": "https://openrouter.ai/api/v1",
      "models": ["google/gemini-2.5-flash", "google/gemini-2.5-pro"]
    }
  },
  "style": "default"
}
```

**Thứ tự tìm kiếm cấu hình** (sau ghi đè trước):

1. `~/.ainovel/config.json` — Cấu hình toàn cục
2. `./.ainovel/config.json` — Cấu hình cấp dự án (tùy chọn)
3. `--config path/to/config.json` — Chỉ định dòng lệnh

> ⚠️ Giá trị của `provider` là **tên key** trong `providers` (một "con trỏ"), không phải tên giao thức. Nếu chuyển `provider` sang một tên không tồn tại trong `providers`, khởi động sẽ báo lỗi "chưa cấu hình thông tin xác thực".

### Sử dụng model khác nhau theo vai trò

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

Các vai trò có thể cấu hình: `coordinator` / `architect` / `writer` / `editor`

### Proxy tùy chỉnh

```jsonc
{
  "provider": "my-proxy",
  "model": "gpt-4o",
  "providers": {
    "my-proxy": {
      "type": "openai",
      "base_url": "https://proxy.example.com/v1"
    }
  }
}
```

Provider được hỗ trợ: `openrouter` / `anthropic` / `gemini` / `openai` / `deepseek` / `qwen` / `glm` / `grok` / `ollama` / `bedrock` và bất kỳ proxy tùy chỉnh nào.

### Ollama cục bộ

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

Chuyển qua trường `style`:

| Giá trị | Mô tả |
|---|---|
| `default` | Phong cách phổ quát |
| `suspense` | Hồi hộp, trinh thám |
| `fantasy` | Giả tưởng, tiên hiệp |
| `romance` | Lãng mạn |

### Quản lý nhiều tiểu thuyết

Mỗi cuốn tiểu thuyết gắn với thư mục làm việc; sản phẩm lưu tại `{cwd}/output/novel/`. Đổi thư mục = đổi tiểu thuyết, chạy lại trong cùng thư mục = tự động khôi phục từ điểm kiểm tra mới nhất.

---

## 📋 Các lệnh TUI

| Lệnh | Chức năng |
|---|---|
| `Ctrl+S` | Bắt đầu / xác nhận sáng tác |
| `/model` | Chuyển đổi model đang dùng |
| `/export` | Xuất tiểu thuyết ra file TXT hoặc EPUB |
| `/import <file>` | Nhập tiểu thuyết có sẵn để tiếp tục |
| `/simulate` | Phân tích văn bản tham khảo, tạo chân dung phong cách |
| `/importsim <file>` | Nhập chân dung phong cách đã tạo sẵn |
| `/diag` | Chạy phân tích chẩn đoán chất lượng |

---

## 📤 Xuất tiểu thuyết

```bash
/export                            # Mặc định TXT, lưu tại {novelDir}/{TênSách}.txt
/export ~/tac-pham.txt             # Xuất ra file TXT chỉ định
/export ~/tac-pham.epub            # Xuất ra EPUB (Apple Books / Kindle)
/export from=10 to=30 --overwrite  # Xuất khoảng chương + ghi đè
```

- **TXT** — Tên sách → Tách tập → Văn bản chương. Tự động loại bỏ tiêu đề trùng lặp từ văn bản gốc.
- **EPUB** — Chuẩn EPUB 3 với trang bìa, mục lục và XHTML phân chương. Tái xuất cùng sách sẽ được reader nhận diện là phiên bản cập nhật.

---

## 📥 Nhập tiểu thuyết có sẵn

```bash
/import ~/tieu-thuyet.txt           # Nhập từ đầu và suy ra foundation
/import ~/tieu-thuyet.txt from=50   # Nhập từ chương 50 (bỏ qua suy ngược)
```

Hệ thống tự nhận dạng các định dạng tiêu đề chương phổ biến:

- Tiếng Việt: `Chương 1`, `Tập 2`, `Phần 3`, `Chương 1: Tựa đề`
- Tiếng Anh: `Chapter 1`, `Chapter II`, `Prologue`, `Epilogue`
- Đặc biệt: `Lời mở đầu`, `Lời kết`, `Ngoại truyện`

---

## 🔄 Khôi phục điểm dừng

Hệ thống tự động khôi phục khi chạy lại cùng thư mục — không cần thao tác thủ công.

| Thời điểm gián đoạn | Hành vi khôi phục |
|---|---|
| Đang lập kế hoạch | Kiểm tra cài đặt đã lưu, tự động hoàn thành phần còn thiếu |
| Đang viết chương | Tiếp tục từ chương đó, đọc bản nháp hiện có |
| Đang đánh giá | Trigger lại biên tập viên |
| Hàng đợi viết lại chưa xóa | Tiếp tục xử lý các chương cần viết lại |
| Mở rộng cung bị gián đoạn | Tự phát hiện và kích hoạt mở rộng Kiến trúc sư |
| Can thiệp người dùng chưa hoàn thành | Inject lại lệnh can thiệp cuối cùng |

> Ghi file sử dụng thao tác nguyên tử (temp + fsync + đổi tên); dữ liệu hiện có không bị hỏng ngay cả khi mất điện.

---

## 🎯 Can thiệp theo thời gian thực

Trong quá trình tạo, nhập nhận xét sửa đổi bất kỳ lúc nào — **không cần dừng hay khởi động lại**.

```
❯ Nâng cao căng thẳng cảm xúc từ chương 4 trở đi, tăng xung đột giữa nhân vật chính và phụ
```

Hệ thống tự động ghi lệnh can thiệp → inject vào Điều phối viên → Điều phối viên đánh giá phạm vi ảnh hưởng và quyết định sửa đổi.

| Lệnh can thiệp | Phản hồi có thể có |
|---|---|
| "Đổi nhân vật chính thành nữ" | Sửa hồ sơ nhân vật, đánh giá cần viết lại chương nào |
| "Chuyển dòng cảm xúc đến chương 4" | Điều chỉnh dàn ý, có thể viết lại từ chương 4 |
| "Thêm nhân vật phản diện" | Cập nhật hồ sơ và quy luật thế giới |
| "Tốc độ quá chậm, tăng tốc" | Điều chỉnh mật độ dàn ý các chương tiếp theo |

---

## 🛠️ Quản lý ngữ cảnh

Khi hội thoại vượt cửa sổ ngữ cảnh, hệ thống nén từng bước:

```
ToolResultMicrocompact → LightTrim → StoreSummaryCompact → FullSummary
  Dọn kết quả công cụ   Cắt văn bản dài  Thay thế bằng tóm tắt Store  Tóm tắt LLM
```

- **StoreSummaryCompact** — Thay trực tiếp tin nhắn cũ bằng tóm tắt chương, trạng thái nhân vật và báo trước trong Store — không tốn chi phí LLM
- **Gói khôi phục nén** — Tự động đưa dàn ý chương hiện tại và trạng thái nhân vật sau FullSummary để tránh "mất trí nhớ"
- **Fuse** — Bỏ qua và hiển thị cảnh báo khi nén liên tục thất bại; tự thử lại ở vòng tiếp theo
- **Chỉ số sức khỏe TUI** — Màu xanh (<70%) → vàng (70-85%) → đỏ (>85%) theo mức sử dụng ngữ cảnh

---

## 📊 Chẩn đoán chất lượng

Nhập `/diag` trong TUI để phân tích chất lượng tiểu thuyết và nhận đề xuất cải tiến có thể thực thi.

| Khía cạnh | Nội dung kiểm tra |
|---|---|
| **Quy trình** | Độ trễ viết lại, hướng dẫn chưa dùng, trạng thái bất thường, bỏ qua chương |
| **Chất lượng** | Điểm thấp liên tiếp, tỉ lệ hoàn thành hợp đồng, tỉ lệ viết lại |
| **Lập kế hoạch** | Báo trước trì trệ, dàn ý cạn kiệt, thiếu tóm tắt |
| **Ngữ cảnh** | Thiếu nhân vật, khoảng trống dòng thời gian, dữ liệu mối quan hệ lỗi thời |

`/diag` cũng xuất file `meta/diag-export.md` — bản giải mẫn cảm chỉ giữ khung hành vi (lời gọi công cụ, chuỗi lỗi, thời gian lặp) mà không có văn bản tiểu thuyết — để dễ dàng báo cáo issue.

---

## 🎨 Loại bỏ hương vị AI & Quy tắc tùy chỉnh

Hệ thống tích hợp sẵn bộ lọc chống AI trong `assets/rules/default.md` và `assets/references/anti-ai-tone.md`.

Để thêm quy tắc riêng, chỉ cần đặt file `.md` trong:

- `~/.ainovel/rules/` — Áp dụng toàn cục
- `./.ainovel/rules/` — Chỉ áp dụng cho cuốn sách hiện tại

Viết quy tắc bằng ngôn ngữ tự nhiên (không cần YAML hay định dạng đặc biệt). Ví dụ:
```
Đừng viết nhân vật chính là người hoàn hảo không có khuyết điểm.
Sử dụng nhận thức cơ thể nhiều hơn thay vì mô tả cảm xúc trực tiếp.
```

Tham khảo [`rules.md.example`](rules.md.example) để xem đầy đủ các trường có thể dùng.

---

## 📁 Cấu trúc thư mục đầu ra

```
output/{ten_tieu_thuyet}/
├── chapters/           # Bản thảo cuối cùng (Markdown)
│   ├── 01.md
│   └── ...
├── summaries/          # Tóm tắt chương (JSON)
├── drafts/             # Bản nháp chương
├── reviews/            # Báo cáo đánh giá
├── meta/
│   ├── premise.md          # Tiền đề câu chuyện
│   ├── outline.json        # Dàn ý phẳng
│   ├── layered_outline.json# Dàn ý phân cấp
│   ├── compass.json        # La bàn hướng kết thúc
│   ├── characters.json     # Hồ sơ nhân vật
│   ├── world_rules.json    # Quy luật thế giới
│   ├── progress.json       # Trạng thái tiến độ
│   ├── foreshadow.json     # Tài khoản báo trước
│   ├── checkpoints.jsonl   # Điểm kiểm tra từng bước
│   └── ...
```

---

## 🔧 Xây dựng từ mã nguồn

```bash
git clone https://github.com/nguyenducvuongg/ainovelViet.git
cd ainovelViet

# Biên dịch
go build ./cmd/ainovel-cli

# Chạy toàn bộ kiểm thử
go test ./...

# Chạy trực tiếp
./ainovel-cli
```

Yêu cầu: **Go 1.21+**

---

## 🧰 Ngăn xếp công nghệ

| Thành phần | Vai trò |
|---|---|
| **Go 1.25** | Ngôn ngữ chính |
| **[agentcore](https://github.com/voocel/agentcore)** | Kernel Agent tối giản (gọi công cụ + phát trực tuyến) |
| **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** | Framework TUI terminal |

---

## 📄 Giấy phép

MIT

---

## 🤝 Đóng góp

Mọi đóng góp đều được chào đón! Vui lòng mở [Issue](https://github.com/nguyenducvuongg/ainovelViet/issues) hoặc [Pull Request](https://github.com/nguyenducvuongg/ainovelViet/pulls).

Dự án này được xây dựng dựa trên [ainovel-cli](https://github.com/voocel/ainovel-cli) gốc và được Việt hóa hoàn toàn để phục vụ cộng đồng người dùng Việt Nam.
