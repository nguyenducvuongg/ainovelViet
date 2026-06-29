---
# Quy tắc mặc định tích hợp của dự án (Phiên bản bảo mật giai đoạn 1)
#
# Chỉ có các ràng buộc mặc định là "có thể kiểm tra bằng máy + ít tranh cãi" được đặt ở đây. Sở thích thẩm mỹ phi máy móc (ví dụ: xu hướng phong cách)
# Hiện vẫn được lưu trữ bởi writer.md / editor.md, sẽ được xác minh ở Giai đoạn 1.5 (kiểm tra tay F1)
# liên kết Working_memory) trước khi quyết định có di chuyển tệp này hay không.
#
# Người dùng có thể ghi đè các trường chung trong thư mục ..ainovel/rules/ hoặc ~/.ainovel/rules/ (bất kỳ .md nào trong đó);
# mệt mỏi_words Hợp nhất theo từ, cùng một từ được bao phủ bởi một nguồn gần hơn theo ngưỡng.
# Để biết ngữ nghĩa trường chi tiết, hãy xem quy tắc gốc của dự án.md.example.

# Phạm vi đếm từ của chương: cảnh báo độ lệch <20%; Lỗi ≥20%.
chapter_words: 3000-6000

# Danh sách đen cụm từ: nếu xuất hiện ≥1 lần là lỗi. trình kiểm tra thực hiện khớp chuỗi con theo nghĩa đen, không có ký tự đại diện,
# Do đó, chỉ đưa vào các công thức AI của "chuỗi có độ dài cố định" (ít gây tranh cãi); các mẫu có biến số (chẳng hạn như "không phải X mà là Y")
# Không thể bắt được kết quả khớp theo nghĩa đen và quay trở lại lớp ngữ nghĩa anti-ai-tone.md.
# Dấu gạch ngang "——" là hợp lệ khi cuộc trò chuyện bị gián đoạn, gây tranh cãi, không nhập mặc định có sẵn và để lại ..ainovel/rules/ để tùy chỉnh.
forbidden_phrases:
  - "ở một mức độ nào đó"
  - "Điều đáng chú ý"
  - "Tôi không biết tại sao"
  - "Hương vị hỗn hợp"

# Giới hạn mềm từ mệt mỏi: commit_chapter sẽ kiểm tra số lần xuất hiện trong mỗi chương và báo cáo cảnh báo nếu vượt quá ngưỡng.
# Đây là những từ thường được sử dụng quá mức trong các bài báo/tiểu thuyết trực tuyến, anti-ai-tone.md có các tín hiệu ngữ nghĩa theo cùng một hướng - tín hiệu hai nguồn nhất quán.
# Sáu mục cuối cùng (như 1/im lặng/không nói/hơi thở X) là từ Chương 196 Chạy đường dài Bằng chứng về sản phẩm: Những lời sáo rỗng truyền thống đã được thay thế bằng những điều trên
# có nghĩa là tuyệt chủng, nhưng thay vào đó, mô hình lại sử dụng những "từ nhịp" này trung bình 5-7 lần; ngưỡng được nới lỏng để chịu đựng việc sử dụng bình thường.
fatigue_words:
  Không thể không: 1
  Trên thực tế: 1
  Như thể: 2
  Ngoài ra: 1
  Tuy nhiên: 2
  Một dấu vết: 2
  Một chạm: 2
  Một nhúm: 2
  Thích: 1
  Không thể không: 1
  Thích một cái :3
  Im lặng: 2
  Không nói nên lời: 2
  Lãi suất: 3
  Một hơi thở: 3
  Lãi suất: 2
---
