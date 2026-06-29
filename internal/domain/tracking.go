package domain

// Bản ghi thay đổi trạng thái vai trò/thực thể StateChange.
type StateChange struct {
	Chapter  int    `json:"chapter"`
	Entity   string `json:"entity"`              // Tên vai trò hoặc tên thực thể
	Field    string `json:"field"`               // Thay đổi thuộc tính: cảnh giới/vị trí/trạng thái/sức mạnh/mối quan hệ, v.v.
	OldValue string `json:"old_value,omitempty"` // Trước khi thay đổi (có giá trị rỗng cho lần xuất hiện đầu tiên)
	NewValue string `json:"new_value"`           // sau khi thay đổi
	Reason   string `json:"reason,omitempty"`    // Lý do thay đổi
}
