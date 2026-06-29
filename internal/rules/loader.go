package rules

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadOptions là các tham số đầu vào của Load.
//
// Việc không tồn tại của tệp không được coi là lỗi và trình tải sẽ bỏ qua nó một cách im lặng; Lỗi phân tích cú pháp không bị chặn và các xung đột được trình phân tích cú pháp ghi vào Parsed.Conflicts.
type LoadOptions struct {
	// RulesFS là cây con nội dung/quy tắc. Người ta đồng ý rằng thư mục gốc chứa trực tiếp default.md.
	// Thường thu được thông qua fs.Sub(embedFS, "rules"); nil có nghĩa là bỏ qua các quy tắc tích hợp.
	RulesFS fs.FS

	// HomeRulesDir là thư mục ~/.ainovel/rules/; trình tải sẽ quét tất cả các tệp .mds cấp cao nhất bên dưới nó (tên tệp được hợp nhất theo thứ tự từ điển). Trống có nghĩa là bỏ qua.
	HomeRulesDir string

	// ProjectRulesDir là thư mục ..ainovel/rules/ (nhân bản toàn cầu, cũng quét tất cả các .mds cấp cao nhất trong đó). Trống có nghĩa là bỏ qua.
	ProjectRulesDir string
}

// Tải các lần đọc theo thứ tự Mặc định → Toàn cầu → Dự án và trả về danh sách Đã phân tích cú pháp được sắp xếp theo thứ tự tăng dần.
//
// Sau khi sáp nhập nhận được giá trị trả về chỉ cần gộp theo thứ tự trong danh sách, cái sau sẽ ghi đè lên cái trước.
// Không có tải ở giai đoạn thứ hai - các lớp mở rộng như Thể loại/Đã học không mở lỗ hổng trước khi có nội dung thực tế.
func Load(opts LoadOptions) []Parsed {
	var layers []Parsed
	if p, ok := readFromFS(opts.RulesFS, "default.md", SourceDefault, "assets/rules/default.md"); ok {
		layers = append(layers, p)
	}
	layers = append(layers, readDirFromDisk(opts.HomeRulesDir, SourceGlobal)...)
	layers = append(layers, readDirFromDisk(opts.ProjectRulesDir, SourceProject)...)
	return layers
}

// readFromFS đọc và phân tích cú pháp từ fs.FS; trả về (Parsed{}, false) nếu tệp không tồn tại.
// displayPath được sử dụng cho Parsed.Source (được hiển thị thuận tiện dưới dạng "nội dung/quy tắc/..." trong nguồn/xung đột).
func readFromFS(fsys fs.FS, name string, kind SourceKind, displayPath string) (Parsed, bool) {
	if fsys == nil {
		return Parsed{}, false
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		// Các tập tin không bị bỏ qua một cách âm thầm; các lỗi khác không bị chặn (trình tải được thiết kế để không báo lỗi)
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return Parsed{}, false
		}
		// Lỗi IO hiếm gặp: hiển thị dưới dạng pars_error, tránh im lặng
		return Parsed{
			Source: displayPath,
			Kind:   kind,
			Conflicts: []Conflict{{
				Source: displayPath,
				Kind:   ConflictParseError,
				Detail: "Đọc không thành công: " + err.Error(),
			}},
		}, true
	}
	return Parse(displayPath, kind, data), true
}

// readFromDisk đọc và phân tích cú pháp từ một đường dẫn tuyệt đối; trả về (Parsed{}, false) khi đường dẫn trống hoặc tệp không tồn tại.
func readFromDisk(absPath string, kind SourceKind) (Parsed, bool) {
	if strings.TrimSpace(absPath) == "" {
		return Parsed{}, false
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Parsed{}, false
		}
		return Parsed{
			Source: absPath,
			Kind:   kind,
			Conflicts: []Conflict{{
				Source: absPath,
				Kind:   ConflictParseError,
				Detail: "Đọc không thành công: " + err.Error(),
			}},
		}, true
	}
	return Parse(absPath, kind, data), true
}

// readDirFromDisk quét tất cả các tệp .md cấp cao nhất trong thư mục (thứ tự từ điển tên tệp) và phân tích từng tệp một thành Parsed.
// Thứ tự từ điển đảm bảo rằng thứ tự hợp nhất của nhiều tệp ở cùng cấp độ là ổn định và có thể dự đoán được (cái sau bao gồm cái trước).
// Bỏ qua các tệp tạm thời bị ẩn/chỉnh sửa trong các thư mục con bắt đầu bằng . (chẳng hạn như macOS ._x.md, emacs .#x.md),
// Tránh đưa nội dung nhị phân của các tệp bẩn vào LLM làm nội dung ưu tiên.
// Nếu đường dẫn hoặc thư mục trống không tồn tại, hàm trả về nil (bị bỏ qua một cách âm thầm, phù hợp với việc thiếu một tệp);
// Thư mục tồn tại nhưng việc đọc không thành công (quyền/đường dẫn thực sự là một tệp) và Xung độtParseError bị lộ và lỗi không được nuốt trong im lặng——
// Phù hợp với hợp đồng chịu lỗi của readFromFS/readFromDisk.
// Không đệ quy vào các thư mục con - giữ cho nó phẳng và tránh đưa ra các hệ thống phân cấp ngầm.
func readDirFromDisk(dir string, kind SourceKind) []Parsed {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []Parsed{{
			Source: dir,
			Kind:   kind,
			Conflicts: []Conflict{{
				Source: dir,
				Kind:   ConflictParseError,
				Detail: "Không đọc được thư mục quy tắc: " + err.Error(),
			}},
		}}
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	var out []Parsed
	for _, name := range names {
		if p, ok := readFromDisk(filepath.Join(dir, name), kind); ok {
			out = append(out, p)
		}
	}
	return out
}

// ainovelDirName là tên dotdir được chia sẻ bởi ainovel ở cả cấp độ người dùng và dự án.
// Do đó, ~/.ainovel/rules/ toàn cầu và dự án ./.ainovel/rules/ có tính đối xứng.
const ainovelDirName = ".ainovel"

// DefaultProjectRulesDir chỉ ra đường dẫn tuyệt đối tới ./.ainovel/rules/ (dựa trên thư mục dự án đã cho).
// Người gọi chuyển vào thư mục gốc của dự án để tránh dựa vào cwd bên trong trình tải; nhân bản DefaultHomeRulesDir.
func DefaultProjectRulesDir(projectDir string) string {
	if projectDir == "" {
		return ""
	}
	return filepath.Join(projectDir, ainovelDirName, "rules")
}

// DefaultHomeRulesDir chỉ ra đường dẫn tuyệt đối đến thư mục ~/.ainovel/rules/.
// Một chuỗi trống được trả về nếu phân tích cú pháp tại nhà không thành công (người gọi sẽ bỏ qua nguồn tương ứng).
func DefaultHomeRulesDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ainovelDirName, "rules")
}

// homeRulesReadme là các hướng dẫn được ghi vào ~/.ainovel/rules/README.txt trong lần khởi động đầu tiên.
// Cố tình sử dụng hậu tố .txt thay vì .md - trình tải chỉ quét .md và mô tả này sẽ không được đưa vào LLM như một quy tắc.
const homeRulesReadme = `Tùy chọn viết chung được đặt ở đây và có hiệu lực trên tất cả các sách.

Cách đơn giản nhất: tạo một tệp .md mới (chẳng hạn như my-style.md) và viết các tùy chọn của bạn bằng tiếng bản địa -
Không cần định dạng, không cần YAML:

    # vai trò
    - Đừng viết nhân vật chính Lin Chen là Đức Trinh Nữ Maria, chỉ lạnh lùng bên ngoài và nóng bỏng bên trong
    #phong cách
    - Sử dụng cảm giác vật lý (đốt ngón tay trắng) thay vì nhãn cảm xúc (lo lắng)
    - Đừng quá hiểu theo nghĩa đen trong cuộc đối thoại của bạn

Những thứ này sẽ được bàn giao cho người biên tập để xem xét ngữ nghĩa. Nhiều .mds được hợp nhất theo thứ tự từ điển theo tên tệp;
Các tệp ẩn và tệp không phải .md bắt đầu bằng dấu chấm sẽ bị bỏ qua (vì vậy README.txt này sẽ không được coi là một quy tắc).

Nâng cao (tùy chọn): Nếu bạn muốn kiểm tra cơ học và cứng nhắc, chẳng hạn như "số từ/từ bị cấm",
Bạn có thể thêm một phần nội dung YAML vào đầu tệp - commit_chapter sẽ đếm nguyên văn và gây ra lỗi:

    ---
    chap_words: 3000-6000 # Phạm vi số từ của chương
    bị cấm_cụm từ: ["Ở một mức độ nào đó"] # Cụm từ bị cấm, sẽ báo lỗi nếu chúng xuất hiện
    mệt mỏi_words: {cannot help: 1} # Mệt mỏi từ ngữ, vượt ngưỡng báo động trong mỗi chương
    ---
    (Viết sở thích của bạn bằng tiếng bản địa như bình thường bên dưới)

Sẽ không thành vấn đề nếu bạn không viết: đường cơ sở cơ học của các cụm từ AI phổ biến và các từ gây mệt mỏi đã được tích hợp sẵn và có thể được sử dụng ngay lập tức.

Ưu tiên tải (cao → thấp): ..ainovel/rules/*.md (cuốn sách này) > ~/.ainovel/rules/*.md (tại đây) > mặc định tích hợp
`

// EnsureHomeRulesDir cố gắng hết sức để tạo thư mục ~/.ainovel/rules/ và viết hướng dẫn README.txt,
// Hãy để người dùng khám phá điểm mở rộng tùy chọn chung này và biết cách viết nó.
// Đường dẫn dễ có, không quan trọng: Lỗi phân tích cú pháp tại nhà hoặc lỗi ghi sẽ được nuốt chửng trong im lặng và sẽ không bao giờ chặn khởi động.
func EnsureHomeRulesDir() {
	if dir := DefaultHomeRulesDir(); dir != "" {
		_ = ensureRulesDirAt(dir)
	}
}

// đảmRulesDirAt tạo một thư mục và ghi README.txt làm mẫu khởi động hiện tại, đây là cốt lõi có thể kiểm tra của EnsureHomeRulesDir.
// README.txt là một tệp khởi động do hệ thống tạo ra (tùy chọn người dùng được viết bằng *.md, không được trình tải tải) và được ghi đè mỗi lần như
// Các mẫu mới nhất - không có nội dung cũ nào được giữ lại, do đó không cần logic tương thích phiên bản.
func ensureRulesDirAt(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.txt"), []byte(homeRulesReadme), 0o644)
}

// DefaultOptions xây dựng LoadOptions chung dựa trên thư mục làm việc hiện tại.
//
// Nó phù hợp để được gọi một lần khi Host khởi động, để ContextTool / CommitChapterTool có thể sử dụng lại cấu hình tương tự.
// ProjectRulesDir bị bỏ trống khi phân tích cú pháp cwd không thành công (trình tải bỏ qua nguồn).
//
// Ngữ nghĩa đường dẫn: ProjectRulesDir liên kết **thư mục làm việc hiện tại (cwd)** thay vì outDir.
// Người dùng cd vào các thư mục khác nhau để bắt đầu viết những cuốn sách khác nhau. .ainovel/rules/ tự nhiên theo sau cwd; nếu việc chia sẻ giữa các cuốn sách là cần thiết,
// Chỉ cần đặt nó vào thư mục chung ~/.ainovel/rules/ (tất cả các tệp .md trong đó sẽ được tải).
func DefaultOptions(rulesFS fs.FS) LoadOptions {
	cwd, _ := os.Getwd()
	return LoadOptions{
		RulesFS:         rulesFS,
		HomeRulesDir:    DefaultHomeRulesDir(),
		ProjectRulesDir: DefaultProjectRulesDir(cwd),
	}
}
