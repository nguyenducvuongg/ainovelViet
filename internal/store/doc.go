// Kho lưu trữ gói cung cấp khả năng lưu trữ liên tục dựa trên hệ thống tệp.
//
// Kiến trúc: 1 cơ sở IO + nhiều kho con + 1 gốc tổng hợp.
// Mỗi kho con chứa một phiên bản IO độc lập và một sync.RWMutex độc lập.
// Đọc và viết ở các lĩnh vực chính (Tiến độ, Đề cương, Bản nháp, Tóm tắt, v.v.) không cản trở nhau;
// WorldStore kết hợp nhiều khu vực nhỏ tần số thấp để chia sẻ một khóa.
//
// Cửa hàng gốc tổng hợp chứa các tham chiếu đến tất cả các cửa hàng con và chịu trách nhiệm về các hoạt động nguyên tử giữa các miền
// （ExpandArc、AppendVolume、ClearHandledSteer）。
//
// Bộ phận lưu trữ phụ:
//   - ProgressStore: Trạng thái chính của tiến trình (meta/progress.json)
//   - OutlineStore: tiền đề, phác thảo (phẳng/lớp), la bàn
//   - DraftStore: Ý tưởng chương, bản nháp, bản nháp cuối cùng
//   - Cửa hàng tóm tắt: Tóm tắt chương/Arc/Tập
//   - RunMetaStore: Chạy siêu dữ liệu (model, lịch sử can thiệp)
//   - SignalStore: file tín hiệu dùng 1 lần (PendingCommit recovery)
//   - CheckpointStore: điểm kiểm tra cấp độ (meta/checkpoints.jsonl)
//   - RuntimeStore: hàng đợi sự kiện thời gian chạy (meta/runtime/*.jsonl)
//   - CharacterStore: file ký tự, ảnh chụp nhanh trạng thái
//   - WorldStore: dòng thời gian, điềm báo, mối quan hệ, thay đổi trạng thái, quy tắc thế giới, quy tắc phong cách, đánh giá
package store
