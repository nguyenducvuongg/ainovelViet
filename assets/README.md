# bản đồ nội dung nội dung

Trước khi thêm "một đoạn/một thông tin/quy tắc" vào hệ thống, trước tiên hãy kiểm tra bảng bên dưới để xác định quyền sở hữu của nó, sau đó xem xét phương pháp nối dây.

| Danh mục | Cài đặt gì | Ai tiêu thụ | Phương pháp nối dây |
|---|---|---|---|
| `prompts/` | Lời nhắc hệ thống vai trò thường trú (điều phối viên/người viết/biên tập viên/kiến trúc sư×2) và lời nhắc nhiệm vụ một lần (nhập×2/mô phỏng×2) | lắp ráp `agents/build.go`; Á hậu imp/sim | Trường Lời nhắc `load.go`. Lưu ý: mô phỏng_guidance được đưa vào khi `load.go` được tải và không thể nhìn thấy trong tệp md |
| `references/` | Tài liệu kiến ​​thức viết theo chủ đề độc lập. Nếu không nhập dấu nhắc hệ thống, tiểu thuyết_context sẽ bị cắt theo vai trò/chương và được đưa vào `reference_pack` | nhà văn / biên tập viên / kiến ​​trúc sư | **Ba kết nối**: Trường thêm `tools.References` + đọc `load.go` LoadReferences + nội dung `novel_context.go` writerReferences / ArchitectReferences. Đưa nó vào thư mục sẽ không tự động tải |
| `references/genres/<style>/` | Kiến thức theo chủ đề cụ thể (tài liệu tham khảo về phong cách / mẫu vòng cung) | Tương tự như trên, được tải khi `style != default` | Tài liệu tham khảo tải `load.go` |
| `rules/` | Giá trị mặc định của quy tắc cơ học (số từ/từ bị cấm/từ mệt mỏi), buộc phải kiểm tra bằng mã khi cam kết | Sáp nhập ba lớp của trình tải quy tắc: tích hợp → `~/.ainovel/rules/` → dự án `./.ainovel/rules/` | `rules/default.md`; định dạng lớp người dùng, xem thư mục gốc `rules.md.example`. Chỉ các chuỗi có độ dài cố định mới được đặt và các mẫu có biến được gửi tới người chỉnh sửa để đánh giá ngữ nghĩa |
| `styles/<style>.md` | Hướng dẫn phong cách viết chủ đề | Nhập lời nhắc hệ thống của **writer** (`agents/build.go`) | Tên tệp là giá trị `config.style`. `references/genres/<style>/` và `references/genres/<style>/` là hai kênh có cùng một khái niệm chủ đề: `references/genres/<style>/` là hướng dẫn về phong cách và `references/genres/<style>/` là tài liệu kiến ​​thức |

## Phán quyết về ghi công nội dung mới (Năm câu hỏi)

1. Quá trình này phải được **đảm bảo**? → Không viết lời nhắc, ghi ràng buộc mã (StopAfterTools/Tool Guard/Flow Router)
2. Đây có phải là tiêu chí quyết định (ai nên cử đi khi nào)? → `prompts/coordinator.md`
3. Đây có phải là tiêu chuẩn thẩm mỹ/điều hành cho một nhân vật nào đó không? → `prompts/<role>.md`
4. Đây có phải là quy tắc đếm được một cách máy móc (từ bị cấm/số từ/ngưỡng) không? → `rules/` (thực thi mã, chi phí LLM bằng 0)
5. Đây có phải là tài liệu kiến ​​thức viết văn không? → `references/` (nhớ kết nối ở 3 nơi)

## Đảm bảo tính nhất quán

Đường dẫn phong bì (`working_memory.*`, v.v.) được tham chiếu bởi dấu nhắc giống với tài liệu tham số commit_chapter của writer.md
Kiểm tra bằng máy `prompts_consistency_test.go` - 2 kiểu drift này không báo lỗi mà chỉ khiến mô hình lặng lẽ trở nên ngu ngốc, bị vạch đỏ của bài thi.
Phân đoạn quy trình được nhắc nhở là "hướng dẫn sử dụng" và sự thật của quy trình nằm ở cấp độ mã; khi có sự ngắt kết nối giữa cả hai, mã sẽ được ưu tiên áp dụng và lời nhắc sẽ được sửa đổi sau đó.
