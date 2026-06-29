package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
)

// Tài liệu tham khảo Tài liệu tham khảo nhúng.
type References struct {
	// V0
	ChapterGuide      string
	HookTechniques    string
	QualityChecklist  string
	OutlineTemplate   string
	CharacterTemplate string
	ChapterTemplate   string
	// V1
	Consistency      string
	ContentExpansion string
	DialogueWriting  string
	// V2
	StyleReference   string // Tham chiếu bổ sung về kiểu dáng (có thể để trống)
	LongformPlanning string // Tài liệu tham khảo quy hoạch dài hạn chung
	Differentiation  string // Tài liệu tham khảo thiết kế khác biệt phổ quát
	ArcTemplates     string // Mẫu vòng cung chủ đề (được tải theo kiểu, có thể để trống)
	AntiAITone       string // Xóa thư viện tiêu chí phù hợp với AI (được chia sẻ bởi người viết/biên tập viên, được chèn đầy đủ)
}

// ContextTool tập hợp ngữ cảnh cần thiết cho chương hiện tại.
type ContextTool struct {
	store     *store.Store
	refs      References
	style     string
	rulesOpts rules.LoadOptions
}

// NewContextTool Tạo một công cụ ngữ cảnh. quy tắcOpts kiểm soát nguồn tải của user_rules;
// LoadOptions trống vẫn an toàn, trình tải bỏ qua tất cả các nguồn chưa được định cấu hình và user_rules tiêm các Gói trống.
func NewContextTool(store *store.Store, refs References, style string, rulesOpts rules.LoadOptions) *ContextTool {
	return &ContextTool{store: store, refs: refs, style: style, rulesOpts: rulesOpts}
}

func (t *ContextTool) Name() string { return "novel_context" }
func (t *ContextTool) Description() string {
	return "Nhận trạng thái hiện tại và bối cảnh tác giả của cuốn tiểu thuyết." +
		"Không vượt qua chương: Trả về tiến trình_status (các trường tiến trình như giai đoạn/luồng/chương_tiếp theo/pending_rewrites) + cài đặt cơ bản, được sử dụng để xác định việc cần làm tiếp theo." +
		"Chương=N: bổ sung trả về tóm tắt của chương, điềm báo, trạng thái nhân vật, quy tắc văn phong và bối cảnh viết khác"
}
func (t *ContextTool) Label() string { return "Tải ngữ cảnh" }

// Một công cụ đọc thuần túy có thể được lên lịch đồng thời.
func (t *ContextTool) ReadOnly(_ json.RawMessage) bool        { return true }
func (t *ContextTool) ConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *ContextTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapter", schema.Int("Số chương. Nếu không vượt qua, trạng thái tiến trình và cài đặt cơ bản sẽ được trả về (được Điều phối viên sử dụng để xác định bước tiếp theo); nếu được truyền vào thì ngữ cảnh viết của chương sẽ được trả về (do Writer sử dụng)")),
	)
}

func (t *ContextTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter int `json:"chapter"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	result := make(map[string]any)
	var warnings []string
	seenWarnings := make(map[string]struct{})
	warn := func(scope string, err error) {
		if err == nil || os.IsNotExist(err) {
			return
		}
		msg := fmt.Sprintf("Đọc %s không thành công: %v", scope, err)
		if _, ok := seenWarnings[msg]; ok {
			return
		}
		seenWarnings[msg] = struct{}{}
		warnings = append(warnings, msg)
	}

	if a.Chapter > 0 {
		// Đường dẫn tác giả: Tải đầy đủ dữ liệu cơ bản + bối cảnh chương
		t.buildBaseContext(result, warn)
		seed := newChapterContextEnvelope()
		state := t.prepareChapterContext(a.Chapter, &seed, warn)
		seed.apply(result)
		t.buildChapterContext(result, state, warn)
		// Chú thích ngữ nghĩa dữ liệu (để đọc lại và giải thích): từng đoạn là một bản ghi nhớ đã được viết thành văn bản, không phải là tài liệu để viết.
		// Chỉ treo nó trong thùng chứa chứ không phải hình ảnh cấp cao nhất.
		if epi, ok := result["episodic_memory"].(map[string]any); ok && len(epi) > 0 {
			epi["_usage"] = "Hộp đựng này là một bản ghi nhớ các sự kiện đã được viết trong văn bản chính (để kiểm soát tính nhất quán và gắn kết); lặp lại những nội dung này như trong nội dung chính của chương mới là lỗi trùng lặp"
		}
	} else {
		// Đường dẫn Điều phối viên/Kiến trúc sư: chỉ trả về trạng thái + dữ liệu có cấu trúc, không tải toàn bộ văn bản gốc
		t.buildProgressStatus(result)
		t.buildArchitectContext(result, warn)
	}

	// Tiêm Working_memory.user_rules (đường dẫn chuẩn). Con đường kiến ​​trúc sư ban đầu không có Working_memory,
	// Tạo vùng chứa chỉ chứa user_rules theo yêu cầu của buildUserRules. Khi RulesOpts trống thì Bundle là một đối tượng trống.
	// Nhưng nó vẫn được xuất ra để tránh LLM nhìn thấy user_rules=null và lấy nhánh bất thường.
	if a.Chapter > 0 {
		t.buildSimulationProfile(result, "working_memory", warn)
	} else {
		t.buildSimulationProfile(result, "planning_memory", warn)
	}

	t.buildUserRules(result)
	t.buildUserDirectives(result, warn)

	if len(warnings) > 0 {
		result["_warnings"] = warnings
	}

	// Ngân sách ưu tiên: Tự động cắt bớt dữ liệu có mức độ ưu tiên thấp khi tổng kích thước vượt quá ngưỡng
	if a.Chapter > 0 {
		trimByBudget(result, 100*1024) // Writer: 100KB
	} else {
		trimByBudget(result, 60*1024) // Coordinator/Architect: 60KB
	}

	result["_loading_summary"] = buildLoadingSummary(result, a.Chapter)
	return json.Marshal(result)
}

// buildLoadingSummary đếm lượng dữ liệu từ kết quả được tập hợp và tạo bản tóm tắt một dòng có thể đọc được.
func buildLoadingSummary(result map[string]any, chapter int) string {
	var parts []string

	if chapter > 0 {
		parts = append(parts, fmt.Sprintf("ch=%d", chapter))
	} else {
		parts = append(parts, "architect")
	}
	if tier, ok := result["planning_tier"].(domain.PlanningTier); ok && tier != "" {
		parts = append(parts, fmt.Sprintf("tier=%s", tier))
	}

	// Vị trí uốn cong
	if pos, ok := result["position"].(map[string]any); ok {
		parts = append(parts, fmt.Sprintf("V%dA%d", pos["volume"], pos["arc"]))
	}

	var items []string
	countSlice := func(key string) int {
		if v, ok := result[key]; ok {
			if s, ok := v.([]domain.Character); ok {
				return len(s)
			}
			// Phản chiếu lát cắt chung
			return sliceLen(v)
		}
		return 0
	}

	// Vai trò
	if n := countSlice("character_snapshots"); n > 0 {
		items = append(items, fmt.Sprintf("Vai trò: %d (ảnh chụp nhanh)", n))
	} else if n := countSlice("characters"); n > 0 {
		items = append(items, fmt.Sprintf("Vai trò:%d", n))
	}

	if working, ok := result["working_memory"].(map[string]any); ok && len(working) > 0 {
		items = append(items, fmt.Sprintf("Bộ nhớ làm việc: %d", len(working)))
	}
	if episodic, ok := result["episodic_memory"].(map[string]any); ok && len(episodic) > 0 {
		items = append(items, fmt.Sprintf("Bộ nhớ tập: %d", len(episodic)))
	}
	if planning, ok := result["planning_memory"].(map[string]any); ok && len(planning) > 0 {
		items = append(items, fmt.Sprintf("Bộ nhớ lập kế hoạch: %d", len(planning)))
	}
	if foundation, ok := result["foundation_memory"].(map[string]any); ok && len(foundation) > 0 {
		items = append(items, fmt.Sprintf("Bộ nhớ cơ bản: %d", len(foundation)))
	}

	// Tóm tắt phân cấp
	if n := countSlice("volume_summaries"); n > 0 {
		items = append(items, fmt.Sprintf("Tóm tắt khối lượng: %d", n))
	}
	if n := countSlice("arc_summaries"); n > 0 {
		items = append(items, fmt.Sprintf("Tóm tắt cung: %d", n))
	}
	if n := countSlice("recent_summaries"); n > 0 {
		items = append(items, fmt.Sprintf("Tóm tắt chương: %d", n))
	}

	// Sơ đồ phân cấp
	if n := countSlice("layered_outline"); n > 0 {
		items = append(items, fmt.Sprintf("Phác thảo lớp: Khối lượng %d", n))
	}

	// dữ liệu trạng thái
	if n := countSlice("timeline"); n > 0 {
		items = append(items, fmt.Sprintf("Dòng thời gian:%d", n))
	}
	if n := countSlice("foreshadow_ledger"); n > 0 {
		items = append(items, fmt.Sprintf("Điềm báo: %d", n))
	}
	if n := countSlice("relationship_state"); n > 0 {
		items = append(items, fmt.Sprintf("Mối quan hệ: %d", n))
	}
	if n := countSlice("recent_state_changes"); n > 0 {
		items = append(items, fmt.Sprintf("Thay đổi trạng thái: %d", n))
	}
	if _, ok := result["previous_tail"]; ok {
		items = append(items, "Kết thúc chương trước: được rồi")
	}
	if _, ok := result["style_rules"]; ok {
		items = append(items, "Quy tắc phong cách: được")
	}
	if n := sliceLen(result["related_chapters"]); n > 0 {
		items = append(items, fmt.Sprintf("Các chương liên quan: %d", n))
	}
	if selected, ok := result["selected_memory"].(map[string]any); ok && len(selected) > 0 {
		if n := sliceLen(selected["story_threads"]); n > 0 {
			items = append(items, fmt.Sprintf("Gợi ý thu hồi: %d", n))
		}
		if n := sliceLen(selected["review_lessons"]); n > 0 {
			items = append(items, fmt.Sprintf("Đánh giá thu hồi:%d", n))
		}
	}

	// Tài liệu tham khảo
	if refs, ok := result["references"].(map[string]string); ok && len(refs) > 0 {
		items = append(items, fmt.Sprintf("Tham khảo: Mục %d", len(refs)))
	}
	if pack, ok := result["reference_pack"].(map[string]any); ok && len(pack) > 0 {
		items = append(items, fmt.Sprintf("Gói tham khảo: %d", len(pack)))
	}
	if _, ok := result["memory_policy"]; ok {
		items = append(items, "Chiến lược ghi nhớ: được")
	}
	if _, ok := result["simulation_profile"]; ok {
		items = append(items, "Chân dung giả: được")
	}
	if warnings, ok := result["_warnings"].([]string); ok && len(warnings) > 0 {
		items = append(items, fmt.Sprintf("Báo động: %d", len(warnings)))
	}
	if trimmed, ok := result["_trimmed"].([]string); ok && len(trimmed) > 0 {
		items = append(items, fmt.Sprintf("Cắt:%s", strings.Join(trimmed, ",")))
	}

	if len(items) > 0 {
		parts = append(parts, strings.Join(items, " "))
	}
	return strings.Join(parts, " | ")
}

// sliceLen cố gắng lấy độ dài lát cắt cho bất kỳ loại nào.
func sliceLen(v any) int {
	switch s := v.(type) {
	case []domain.ChapterSummary:
		return len(s)
	case []domain.ArcSummary:
		return len(s)
	case []domain.VolumeSummary:
		return len(s)
	case []domain.CharacterSnapshot:
		return len(s)
	case []domain.TimelineEvent:
		return len(s)
	case []domain.ForeshadowEntry:
		return len(s)
	case []domain.RelationshipEntry:
		return len(s)
	case []domain.StateChange:
		return len(s)
	case []domain.VolumeOutline:
		return len(s)
	case []domain.Character:
		return len(s)
	case []domain.RelatedChapter:
		return len(s)
	case []domain.RecallItem:
		return len(s)
	default:
		return 0
	}
}

// LoadFilteredCharacters Lọc các ký tự theo Cấp độ và hình thức cảnh.
// cốt lõi/quan trọng luôn được trả về; thứ cấp/trang trí chỉ được trả về khi được đề cập trong dàn ý chương hiện tại.
func (t *ContextTool) loadFilteredCharacters(result map[string]any, chapter int, warn func(string, error)) {
	chars, err := t.store.Characters.Load()
	if err != nil {
		warn("characters", err)
		return
	}
	if len(chars) == 0 {
		return
	}

	// Lấy mô tả cảnh của dàn ý chương hiện tại, dùng để khớp với các ký tự phụ
	entry, err := t.store.Outline.GetChapterOutline(chapter)
	if err != nil {
		warn("current_chapter_outline", err)
		result["characters"] = chars
		return
	}
	sceneText := strings.Join(entry.Scenes, " ") + " " + entry.CoreEvent + " " + entry.Title

	var filtered []domain.Character
	for _, c := range chars {
		switch c.Tier {
		case "secondary", "decorative":
			if matchCharacter(sceneText, c) {
				filtered = append(filtered, c)
			}
		default: // cốt lõi, quan trọng hoặc chưa được thiết lập
			filtered = append(filtered, c)
		}
	}
	result["characters"] = filtered
}

// matchCharacter Kiểm tra xem văn bản cảnh có chứa tên chính thức của nhân vật hay bất kỳ bí danh nào của nhân vật đó hay không.
func matchCharacter(text string, c domain.Character) bool {
	if strings.Contains(text, c.Name) {
		return true
	}
	for _, alias := range c.Aliases {
		if strings.Contains(text, alias) {
			return true
		}
	}
	return false
}

// LoadLayeredSummaries Tải các bản tóm tắt theo lớp: tóm tắt tập + tóm tắt phần tập hiện tại + tóm tắt chương trong phần.
func (t *ContextTool) loadLayeredSummaries(result map[string]any, chapter, summaryWindow int, warn func(string, error)) {
	vol, arc, err := t.store.Outline.LocateChapter(chapter)
	if err != nil {
		warn("layered_outline_position", err)
		// Trở lại chế độ phẳng
		if summaries, err := t.store.Summaries.LoadRecentSummaries(chapter, summaryWindow); err == nil && len(summaries) > 0 {
			result["recent_summaries"] = summaries
		} else {
			warn("recent_summaries", err)
		}
		return
	}

	// 1. Tổng hợp các tập đã hoàn thành
	if volSummaries, err := t.store.Summaries.LoadAllVolumeSummaries(); err == nil && len(volSummaries) > 0 {
		result["volume_summaries"] = volSummaries
	} else {
		warn("volume_summaries", err)
	}

	// 2. Tóm tắt các cung đã hoàn thành trong tập hiện tại (không bao gồm cung hiện tại)
	if arcSummaries, err := t.store.Summaries.LoadArcSummaries(vol); err == nil && len(arcSummaries) > 0 {
		var prior []domain.ArcSummary
		for _, s := range arcSummaries {
			if s.Arc < arc {
				prior = append(prior, s)
			}
		}
		if len(prior) > 0 {
			result["arc_summaries"] = prior
		}
	} else {
		warn("arc_summaries", err)
	}

	// 3. Tóm tắt chương của N chương gần đây nhất trong phần hiện tại
	if summaries, err := t.store.Summaries.LoadRecentSummaries(chapter, summaryWindow); err == nil && len(summaries) > 0 {
		result["recent_summaries"] = summaries
	} else {
		warn("recent_summaries", err)
	}
}

// LoadLayeredCharacters Tải ký tự ở chế độ Lớp: Ưu tiên ảnh chụp nhanh mới nhất, quay lại cài đặt gốc + Lọc theo cấp.
func (t *ContextTool) loadLayeredCharacters(result map[string]any, chapter int, warn func(string, error)) {
	snapshots, err := t.store.Characters.LoadLatestSnapshots()
	if err == nil && len(snapshots) > 0 {
		result["character_snapshots"] = snapshots
		// Đồng thời giữ lại các vai trò cốt lõi/quan trọng trong cài đặt gốc (ảnh chụp nhanh có thể không chứa các ký tự mới)
		t.loadFilteredCharacters(result, chapter, warn)
		return
	}
	warn("character_snapshots", err)
	// Hoàn nguyên về cài đặt gốc khi không có ảnh chụp nhanh
	t.loadFilteredCharacters(result, chapter, warn)
}

// writerReferences Trả về việc viết tài liệu tham khảo. Chương 1 quay lại kích thước đầy đủ và các chương tiếp theo sẽ cắt bỏ các mẫu không còn cần thiết nữa.
func (t *ContextTool) writerReferences(chapter int) map[string]string {
	refs := map[string]string{}
	add := func(k, v string) {
		if v != "" {
			refs[k] = v
		}
	}
	// Tải dần dần: các tài liệu tham khảo cốt lõi luôn được giữ lại, 3 chương đầu tiên được tải bổ sung với hướng dẫn viết đầy đủ
	add("consistency", t.refs.Consistency)
	add("hook_techniques", t.refs.HookTechniques)
	add("quality_checklist", t.refs.QualityChecklist)
	add("anti_ai_tone", t.refs.AntiAITone) // Các tiêu chí phù hợp với AI được đưa vào trong suốt quá trình và không bị cắt theo các chương.
	if chapter <= 3 {
		add("chapter_guide", t.refs.ChapterGuide)
		add("dialogue_writing", t.refs.DialogueWriting)
		add("style_reference", t.refs.StyleReference)
	}

	// Tài liệu tham khảo bổ sung chỉ được tải cho chương đầu tiên
	if chapter <= 1 {
		add("chapter_template", t.refs.ChapterTemplate)
		add("content_expansion", t.refs.ContentExpansion)
	}
	return refs
}

func (t *ContextTool) architectReferences() map[string]string {
	refs := map[string]string{}
	add := func(k, v string) {
		if v != "" {
			refs[k] = v
		}
	}
	add("outline_template", t.refs.OutlineTemplate)
	add("character_template", t.refs.CharacterTemplate)
	add("longform_planning", t.refs.LongformPlanning)
	add("differentiation", t.refs.Differentiation)
	add("style_reference", t.refs.StyleReference)
	add("arc_templates", t.refs.ArcTemplates)
	add("anti_ai_tone", t.refs.AntiAITone) // Phác thảo kiến ​​trúc loại bỏ giọng AI; người biên tập cũng sử dụng đường dẫn Chapter=0
	return refs
}

// FoundationStatus kiểm tra tính đầy đủ của cài đặt nền tảng và trả về danh sách các mục bị thiếu.
// Chia sẻ logic quyết định store.FoundationThiếu bằng công cụ save_foundation để đảm bảo rằng LLM không bao giờ
// sẵn sàng/thiếu được xem bởi tiểu thuyết_context và Foundation_ready được trả về bởi save_foundation
// Luôn nhất quán (các chi tiết như các yếu tố cần thiết của la bàn dạng dài không bị trôi).
func (t *ContextTool) foundationStatus() map[string]any {
	missing := t.store.FoundationMissing()
	status := map[string]any{"ready": len(missing) == 0}
	if len(missing) > 0 {
		status["missing"] = missing
	}
	return status
}

// ContextSummary Trả về bản tóm tắt ngắn gọn về trạng thái hiện tại (cho mục đích ghi nhật ký).
func (t *ContextTool) ContextSummary() string {
	var parts []string
	if p, _ := t.store.Outline.LoadPremise(); p != "" {
		parts = append(parts, "premise:ok")
	}
	if o, _ := t.store.Outline.LoadOutline(); o != nil {
		parts = append(parts, fmt.Sprintf("outline:%d chapters", len(o)))
	}
	if c, _ := t.store.Characters.Load(); c != nil {
		parts = append(parts, fmt.Sprintf("characters:%d", len(c)))
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, ", ")
}

// TrimByBudget cắt bớt kết quả theo mức độ ưu tiên để tổng kích thước JSON không vượt quá byte ngân sách.
// Mức độ ưu tiên (thấp hơn đến cao hơn): tài liệu tham khảo < voice_samples < style_anchors < previous_tail < dòng thời gian
//
//	< gần đây_state_changes < foreshadow_ledger < mối quan hệ_state < phần còn lại (không bị cắt bớt)
//
// Khóa đã cắt sẽ được ghi lại trong result["_trimmed"] để khắc phục sự cố nhật ký.
func trimByBudget(result map[string]any, budget int) {
	// Đầu tiên đo kích thước hiện tại
	data, err := json.Marshal(result)
	if err != nil || len(data) <= budget {
		return
	}

	// Liệt kê các khóa có thể cắt tỉa từ mức độ ưu tiên thấp đến cao
	trimOrder := []string{
		"references",
		"voice_samples",
		"style_anchors",
		"style_rules",
		"style_stats",
		"previous_tail",
		"timeline",
		"recent_state_changes",
		"foreshadow_ledger",
		"relationship_state",
	}

	var trimmed []string
	for _, key := range trimOrder {
		if _, ok := result[key]; !ok {
			continue
		}
		deleteContextKey(result, key)
		trimmed = append(trimmed, key)
		data, err = json.Marshal(result)
		if err != nil || len(data) <= budget {
			break
		}
	}
	if len(trimmed) > 0 {
		result["_trimmed"] = trimmed
	}
}

func deleteContextKey(result map[string]any, key string) {
	delete(result, key)
	for _, containerKey := range []string{
		"working_memory",
		"episodic_memory",
		"planning_memory",
		"foundation_memory",
		"reference_pack",
	} {
		section, ok := result[containerKey].(map[string]any)
		if !ok {
			continue
		}
		delete(section, key)
	}
}

// buildRelatChapters Truy xuất các chương lịch sử liên quan đến chương hiện tại dựa trên dữ liệu có cấu trúc.
// Các khuyến nghị được đưa ra từ bốn khía cạnh: điềm báo, ngoại hình nhân vật, thay đổi trạng thái và các mối quan hệ. Tối đa 5 mục sẽ được trả lại sau khi loại bỏ trùng lặp.
// Tất cả dữ liệu được truyền qua các tham số và không có IO bổ sung nào được thực hiện.
func (t *ContextTool) buildRelatedChapters(
	chapter int,
	entry *domain.OutlineEntry,
	foreshadow []domain.ForeshadowEntry,
	relationships []domain.RelationshipEntry,
	stateChanges []domain.StateChange,
) []domain.RelatedChapter {
	const recentWindow = 10
	const maxResults = 5

	seen := make(map[int]struct{})
	var results []domain.RelatedChapter
	add := func(ch int, reason string) {
		if ch <= 0 || ch >= chapter {
			return
		}
		// Một số chương cuối quá gần đây và không được khuyến khích.
		if ch > chapter-recentWindow {
			return
		}
		if _, ok := seen[ch]; ok {
			return
		}
		seen[ch] = struct{}{}
		results = append(results, domain.RelatedChapter{Chapter: ch, Reason: reason})
	}

	// Nối văn bản phác thảo để khớp từ khóa
	outlineText := entry.Title + " " + entry.CoreEvent
	for _, s := range entry.Scenes {
		outlineText += " " + s
	}

	// 1. Kiểm tra lại điềm báo: liệu mô tả về điềm báo tích cực có liên quan đến dàn ý của chương hiện tại hay không
	for _, f := range foreshadow {
		if strings.Contains(outlineText, f.ID) || containsAny(outlineText, strings.Fields(f.Description)) {
			add(f.PlantedAt, fmt.Sprintf("Báo trước chương chôn vùi %s (%s)", f.ID, truncateRunes(f.Description, 15)))
		}
		if len(results) >= maxResults {
			break
		}
	}

	// 2. Kiểm tra ngược hình thức ký tự: duyệt đơn hàng loạt, IO giảm từ O (số ký tự × số chương) xuống O (số chương)
	chars, _ := t.store.Characters.Load()
	outlineChars := matchOutlineCharacters(outlineText, chars)
	if len(outlineChars) > 0 {
		appearances := t.store.Summaries.FindCharacterAppearances(outlineChars, chapter, recentWindow)
		for _, name := range outlineChars {
			if len(results) >= maxResults {
				break
			}
			if ch, ok := appearances[name]; ok {
				add(ch, fmt.Sprintf("Chương cuối của nhân vật '%s'", name))
			}
		}
	}

	// 3. Kiểm tra ngược thay đổi trạng thái: hoạt động trên lát được tải, IO bằng 0
	for _, name := range outlineChars {
		if len(results) >= maxResults {
			break
		}
		ch := findLastStateChange(stateChanges, name, chapter)
		if ch > 0 && ch <= chapter-recentWindow {
			add(ch, fmt.Sprintf("Chương thay đổi trạng thái '%s'", name))
		}
	}

	// 4. Xem lại mối quan hệ: sự thay đổi cuối cùng trong mối quan hệ giữa các cặp nhân vật có liên quan đến chương hiện tại
	if len(relationships) > 0 && len(outlineChars) >= 2 {
		charSet := make(map[string]struct{}, len(outlineChars))
		for _, c := range outlineChars {
			charSet[c] = struct{}{}
		}
		for _, r := range relationships {
			if len(results) >= maxResults {
				break
			}
			_, aIn := charSet[r.CharacterA]
			_, bIn := charSet[r.CharacterB]
			if aIn && bIn {
				add(r.Chapter, fmt.Sprintf("Những thay đổi trong mối quan hệ %s-%s", r.CharacterA, r.CharacterB))
			}
		}
	}

	return results
}

// findLastStateChange tìm số chương của thay đổi cuối cùng của thực thể trong danh sách thay đổi trạng thái được tải.
func findLastStateChange(changes []domain.StateChange, entity string, currentChapter int) int {
	for i := len(changes) - 1; i >= 0; i-- {
		if changes[i].Entity == entity && changes[i].Chapter < currentChapter {
			return changes[i].Chapter
		}
	}
	return 0
}

// matchOutlineCharacters So khớp tên ký tự xuất hiện từ văn bản phác thảo.
func matchOutlineCharacters(text string, chars []domain.Character) []string {
	var matched []string
	for _, c := range chars {
		if strings.Contains(text, c.Name) {
			matched = append(matched, c.Name)
			continue
		}
		for _, alias := range c.Aliases {
			if strings.Contains(text, alias) {
				matched = append(matched, c.Name)
				break
			}
		}
	}
	return matched
}

// containsAny kiểm tra xem văn bản có chứa bất kỳ từ nào trong từ hay không (phải khớp ít nhất 2 từ để tránh nhiễu).
func containsAny(text string, words []string) bool {
	for _, w := range words {
		if len([]rune(w)) >= 2 && strings.Contains(text, w) {
			return true
		}
	}
	return false
}

func (t *ContextTool) selectStoryThreads(state contextBuildState) []domain.RecallItem {
	if state.currentEntry == nil {
		return nil
	}
	if len(state.foreshadow) < storyThreadRecallThreshold {
		return nil
	}

	const maxThreads = 5
	var items []domain.RecallItem
	seen := make(map[string]struct{})
	picked := make(map[string]struct{}) // ID báo trước đã chọn được sử dụng để chèn lấp cũ và loại bỏ trùng lặp.
	add := func(item domain.RecallItem) {
		key := item.Kind + "|" + item.Key + "|" + item.Summary
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		picked[item.Key] = struct{}{}
		items = append(items, item)
	}

	// 1. Nhớ lại sự liên quan: điềm báo trùng lặp với từ trọng tâm trong chương hiện tại.
	focusTerms := recallFocusTerms(state.currentEntry, state.chapterPlan)
	focusText := strings.Join(focusTerms, " ")
	for _, entry := range state.foreshadow {
		if !matchesRecallTerms(entry.ID+" "+entry.Description, focusTerms) && !strings.Contains(focusText, entry.ID) {
			continue
		}
		add(domain.RecallItem{
			Kind:    "story_thread",
			Key:     entry.ID,
			Chapter: entry.PlantedAt,
			Reason:  "Chương hiện tại có thể cần phải tiếp nối những điềm báo hiện có",
			Summary: fmt.Sprintf("Điềm báo “%s” bị chôn vùi trong Chương %d: %s", entry.ID, entry.PlantedAt, truncateRunes(entry.Description, 30)),
		})
		if len(items) >= maxThreads {
			return items
		}
	}

	// 2. Lão hóa chèn lấp: Điềm báo không liên quan gì đến chương hiện tại nhưng đã chờ xử lý từ lâu (cũ nhất trước) sẽ chiếm hạn ngạch còn lại.
	//    Điều đang được bổ sung là điểm mù tự nhiên của việc nhớ lại mối tương quan - sợi chỉ treo một mình quá lâu mà không chạm tới các từ khóa trong chương này.
	for _, entry := range agingForeshadow(state.foreshadow, state.chapter, picked) {
		add(domain.RecallItem{
			Kind:    "story_thread",
			Key:     entry.ID,
			Chapter: entry.PlantedAt,
			Reason:  "Lời báo trước đã chờ đợi từ lâu và vẫn chưa được phục hồi. Hãy chú ý để tiến lên hoặc phục hồi nó kịp thời.",
			Summary: fmt.Sprintf("Điềm báo “%s” bị chôn vùi trong Chương %d, Chương %d chưa tìm lại được: %s", entry.ID, entry.PlantedAt, state.chapter-entry.PlantedAt, truncateRunes(entry.Description, 30)),
		})
		if len(items) >= maxThreads {
			break
		}
	}

	return items
}

// ageForeshadow trả về các phần báo trước chưa được phục hồi với age ≥ foreshadowAgingChapters, được sắp xếp theo thứ tự cũ nhất trước tiên.
// Bỏ qua những lựa chọn được chọn bằng cách thu hồi tương quan. Tất cả tham số đầu vào đều đã là danh sách đang hoạt động (không được tái chế), do đó không cần phải lọc trạng thái.
func agingForeshadow(all []domain.ForeshadowEntry, chapter int, picked map[string]struct{}) []domain.ForeshadowEntry {
	var aging []domain.ForeshadowEntry
	for _, e := range all {
		if _, ok := picked[e.ID]; ok {
			continue
		}
		if e.PlantedAt <= 0 || chapter-e.PlantedAt < foreshadowAgingChapters {
			continue
		}
		aging = append(aging, e)
	}
	sort.SliceStable(aging, func(i, j int) bool {
		return aging[i].PlantedAt < aging[j].PlantedAt
	})
	return aging
}

func (t *ContextTool) selectReviewLessons(chapter int, warn func(string, error)) []domain.RecallItem {
	if chapter <= 1 {
		return nil
	}

	var items []domain.RecallItem
	seen := make(map[string]struct{})
	add := func(item domain.RecallItem) {
		key := item.Summary
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}

	appendReview := func(review *domain.ReviewEntry) bool {
		if review == nil {
			return false
		}
		for i, miss := range review.ContractMisses {
			add(domain.RecallItem{
				Kind:    "review_lesson",
				Key:     fmt.Sprintf("review-%d-contract-%d", review.Chapter, i),
				Chapter: review.Chapter,
				Reason:  "Một đánh giá gần đây đã chỉ ra rằng hợp đồng thiếu các mục",
				Summary: fmt.Sprintf("Chương %d hợp đồng thiếu mục: %s", review.Chapter, miss),
			})
			if len(items) >= 3 {
				return true
			}
		}
		for i, issue := range review.Issues {
			switch issue.Severity {
			case "", "warning", "error", "critical":
				add(domain.RecallItem{
					Kind:    "review_lesson",
					Key:     fmt.Sprintf("review-%d-issue-%d", review.Chapter, i),
					Chapter: review.Chapter,
					Reason:  "Một đánh giá gần đây lưu ý sự cần thiết phải tránh sự trùng lặp của các vấn đề",
					Summary: fmt.Sprintf("Lời nhắc ôn tập chương %d: %s", review.Chapter, truncateRunes(issue.Description, 36)),
				})
			}
			if len(items) >= 3 {
				return true
			}
		}
		return false
	}

	for ch := chapter - 1; ch >= max(chapter-3, 1); ch-- {
		review, err := t.store.World.LoadReview(ch)
		if err != nil {
			warn("review", err)
			continue
		}
		if appendReview(review) {
			return items
		}
	}

	globalReview, err := t.store.World.LoadLastReview(chapter - 1)
	if err != nil {
		warn("global_review", err)
	} else if appendReview(globalReview) {
		return items
	}
	return items
}

func recallFocusTerms(entry *domain.OutlineEntry, plan *domain.ChapterPlan) []string {
	if entry == nil {
		return nil
	}
	var terms []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" {
			terms = append(terms, v)
		}
	}

	add(entry.Title)
	add(entry.CoreEvent)
	add(entry.Hook)
	for _, scene := range entry.Scenes {
		add(scene)
	}
	if plan != nil {
		add(plan.Goal)
		add(plan.Hook)
		for _, point := range plan.Contract.PayoffPoints {
			add(point)
		}
		add(plan.Contract.HookGoal)
	}
	return terms
}

func matchesRecallTerms(text string, terms []string) bool {
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if len([]rune(term)) < 2 {
			continue
		}
		if strings.Contains(text, term) || strings.Contains(term, text) {
			return true
		}
		if hasMeaningfulOverlap(term, text) {
			return true
		}
	}
	return false
}

func hasMeaningfulOverlap(a, b string) bool {
	ar := []rune(strings.TrimSpace(a))
	br := []rune(strings.TrimSpace(b))
	if len(ar) < 5 || len(br) < 5 {
		return false
	}
	shorter := len(ar)
	if len(br) < shorter {
		shorter = len(br)
	}
	threshold := 5
	switch {
	case shorter >= 12:
		threshold = 7
	case shorter >= 9:
		threshold = 6
	}
	return longestCommonSubstringRunes(ar, br) >= threshold
}

const storyThreadRecallThreshold = 6
const storyThreadRecallMinSelected = 2

// foreshadowAgingChapters: Một đoạn báo trước chưa được tái sử dụng sau nhiều chương này đã được coi là "lâu đời".
// Ngay cả khi loại điềm báo này không liên quan gì đến từ khóa của chương hiện tại, nó sẽ được điền lại vào story_threads để tránh bị lãng quên hoàn toàn trong câu chuyện dài.
// (Thu hồi liên quan tự nhiên chỉ nhìn thấy những chủ đề liên quan đến chương này, không thể nhìn thấy chủ đề đã treo quá lâu).
// Thực tế là quá trình lão hóa tài khoản bắt nguồn từ mã thuần túy (chương hiện tại - chương bị chôn vùi) chỉ nêu rõ "N chương đã bị treo nhưng không được tái chế" mà không đưa ra hướng dẫn.
const foreshadowAgingChapters = 30

func longestCommonSubstringRunes(a, b []rune) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	prev := make([]int, len(b)+1)
	best := 0
	for i := 1; i <= len(a); i++ {
		curr := make([]int, len(b)+1)
		for j := 1; j <= len(b); j++ {
			if a[i-1] != b[j-1] {
				continue
			}
			curr[j] = prev[j-1] + 1
			if curr[j] > best {
				best = curr[j]
			}
		}
		prev = curr
	}
	return best
}

// truncateRunes cắt ngắn một chuỗi theo số lần chạy đã chỉ định.
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}
