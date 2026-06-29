package store

import (
	"os"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// UsageStore duy trì mức sử dụng tích lũy mã thông báo/chi phí cho meta/usage.json.
// Viết thông qua ghi nguyên tử của IO (tmp + đổi tên) và đường dẫn Lưu bao phủ hoàn toàn toàn bộ trạng thái mỗi lần.
type UsageStore struct{ io *IO }

func NewUsageStore(io *IO) *UsageStore { return &UsageStore{io: io} }

// Tải lượt đọc use.json. Trả về (nil, nil) khi tệp không tồn tại hoặc phiên bản lược đồ không khớp,
// Người gọi có thể quyết định xem có sử dụng tính năng phát lại phiên cho chèn lấp một lần hay không.
func (s *UsageStore) Load() (*domain.UsageState, error) {
	var state domain.UsageState
	if err := s.io.ReadJSON("meta/usage.json", &state); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if state.Schema != domain.UsageSchemaVersion {
		return nil, nil
	}
	return &state, nil
}

// Lưu ghi đè hoàn toàn trạng thái và ghi nó vào đĩa. Người gọi có trách nhiệm gỡ lỗi/điều tiết.
func (s *UsageStore) Save(state domain.UsageState) error {
	state.Schema = domain.UsageSchemaVersion
	return s.io.WriteJSON("meta/usage.json", state)
}
