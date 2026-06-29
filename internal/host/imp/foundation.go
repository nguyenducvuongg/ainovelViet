package imp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// FoundationResult là sản phẩm có cấu trúc đảo ngược của Foundation.
type FoundationResult struct {
	Premise    string                 // Chuỗi đánh dấu
	Characters []domain.Character     // hồ sơ nhân vật
	WorldRules []domain.WorldRule     // quy tắc thế giới
	Volumes    []domain.VolumeOutline // Phác thảo phân cấp: nhập văn bản chính làm tập đầu tiên (có thể tái tạo và mở rộng)
	Compass    *domain.StoryCompass   // Điểm neo hướng tiếp tục (ending_direction / open_threads / Estimate_scale)
}

// LLMChat là phần phụ thuộc tối thiểu của gói imp vào ChatModel: chỉ cần tạo một văn bản thông thường.
// Việc trích xuất các giao diện độc lập tạo điều kiện thuận lợi cho việc đưa các mô hình vào các thử nghiệm đơn lẻ và tránh việc ghép nối trực tiếp với máy khách Agentcore.
type LLMChat interface {
	Generate(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error)
}

// ReverseFoundation sử dụng một lệnh gọi LLM duy nhất để đảo ngược nền tảng từ văn bản chương được phân đoạn.
// Không gọi save_foundation, hàm thuần túy; sự kiên trì được xác định bởi người gọi.
func ReverseFoundation(ctx context.Context, llm LLMChat, systemPrompt string, chapters []Chapter) (*FoundationResult, error) {
	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapters to analyze")
	}
	if llm == nil {
		return nil, fmt.Errorf("llm is nil")
	}

	system := strings.ReplaceAll(systemPrompt, "${chapter_count}", fmt.Sprintf("%d", len(chapters)))
	user := buildFoundationUserPrompt(chapters)

	resp, err := llm.Generate(ctx, []agentcore.Message{
		agentcore.SystemMsg(system),
		agentcore.UserMsg(user),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("llm generate: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("llm returned nil response")
	}

	return parseFoundationOutput(resp.Message.TextContent(), len(chapters))
}

// buildFoundationUserPrompt Mẹo dành cho người dùng hội: Tất cả các chương được ghép theo thứ tự và các neo số chương được đính kèm để dễ dàng tham khảo LLM.
func buildFoundationUserPrompt(chapters []Chapter) string {
	var sb strings.Builder
	sb.WriteString("Sau đây là những gì đã được thực hiện ")
	fmt.Fprintf(&sb, "%d", len(chapters))
	sb.WriteString(" Văn bản chương. Vui lòng tuân thủ nghiêm ngặt lời nhắc của hệ thống để đảo ngược nền tảng và xuất ra năm phân đoạn === TAG ===. \n\n")
	for i, ch := range chapters {
		fmt.Fprintf(&sb, "## Chương %d: %s\n\n", i+1, ch.Title)
		sb.WriteString(ch.Content)
		sb.WriteString("\n\n---\n\n")
	}
	return sb.String()
}

// parsFoundationOutput Phân tích đường bao đầu ra LLM và xác minh các ràng buộc chính.
func parseFoundationOutput(text string, expectChapters int) (*FoundationResult, error) {
	env := parseTaggedEnvelope(text)
	if env == nil {
		return nil, fmt.Errorf("no === TAG === envelope found in LLM output")
	}
	if err := requireTags(env, "PREMISE", "CHARACTERS", "WORLD_RULES", "LAYERED_OUTLINE", "COMPASS"); err != nil {
		return nil, err
	}

	premise := stripFences(env["PREMISE"])
	if !strings.HasPrefix(strings.TrimLeft(premise, " \t\n"), "#") {
		return nil, fmt.Errorf("tiền đề phải bắt đầu bằng dòng tiêu đề Markdown (# tên sách)")
	}

	var characters []domain.Character
	if err := decodeJSON("characters", env["CHARACTERS"], &characters); err != nil {
		return nil, err
	}
	if len(characters) == 0 {
		return nil, fmt.Errorf("characters array is empty")
	}

	var worldRules []domain.WorldRule
	if err := decodeJSON("world_rules", env["WORLD_RULES"], &worldRules); err != nil {
		return nil, err
	}

	var volumes []domain.VolumeOutline
	if err := decodeJSON("layered_outline", env["LAYERED_OUTLINE"], &volumes); err != nil {
		return nil, err
	}
	// Khi nhập dàn bài, tất cả N chương phải được mở rộng (FlattenOutline chỉ tính các chương thực, không tính các cung khung),
	// Ngược lại, khi thực hiện từng chương, một số chương sẽ nằm ngoài phạm vi phác thảo và bị người bảo vệ ngoài giới hạn từ chối.
	if got := len(domain.FlattenOutline(volumes)); got != expectChapters {
		return nil, fmt.Errorf("layered outline chapter count mismatch: got %d, want %d", got, expectChapters)
	}

	var compass domain.StoryCompass
	if err := decodeJSON("compass", env["COMPASS"], &compass); err != nil {
		return nil, err
	}

	return &FoundationResult{
		Premise:    premise,
		Characters: characters,
		WorldRules: worldRules,
		Volumes:    volumes,
		Compass:    &compass,
	}, nil
}

// PersistFoundation ghi kết quả suy luận vào Cửa hàng theo thứ tự giống như lời nhắc dài của Kiến trúc sư:
// tiền đề → ký tự → world_rules → layered_outline → la bàn. Nhập văn bản làm tập đầu tiên
// Nằm trong một dàn bài có thứ bậc để có thể tiếp tục và mở rộng sách đã nhập. Mỗi bước sẽ kích hoạt logic save_foundation của việc đặt cùng một mô hình.
//
// SaveFoundationTool không được gọi trực tiếp vì đây là hoạt động phát lại xác định và không yêu cầu lập lịch công cụ LLM.
// Nhưng vẫn giữ các tác dụng phụ tương tự như SaveFoundationTool: tiến giai đoạn, thêm điểm kiểm tra.
func PersistFoundation(ctx context.Context, st *store.Store, scale domain.PlanningTier, fr *FoundationResult) error {
	if fr == nil {
		return fmt.Errorf("nil foundation result")
	}
	if err := st.RunMeta.SetPlanningTier(scale); err != nil {
		return fmt.Errorf("save planning tier: %w", err)
	}

	// 1. premise
	if err := st.Outline.SavePremise(fr.Premise); err != nil {
		return fmt.Errorf("save premise: %w", err)
	}
	if name := domain.ExtractNovelNameFromPremise(fr.Premise); name != "" {
		_ = st.Progress.SetNovelName(name)
	}
	_ = st.Progress.UpdatePhase(domain.PhasePremise)
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "premise", "premise.md"); err != nil {
		return fmt.Errorf("checkpoint premise: %w", err)
	}

	// 2. characters
	if err := st.Characters.Save(fr.Characters); err != nil {
		return fmt.Errorf("save characters: %w", err)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "characters", "characters.json"); err != nil {
		return fmt.Errorf("checkpoint characters: %w", err)
	}

	// 3. world_rules
	if err := st.World.SaveWorldRules(fr.WorldRules); err != nil {
		return fmt.Errorf("save world_rules: %w", err)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "world_rules", "world_rules.json"); err != nil {
		return fmt.Errorf("checkpoint world_rules: %w", err)
	}

	// 4. phác thảo theo lớp (nhập văn bản dưới dạng tập đầu tiên → chế độ xếp lớp, có thể tiếp tục và có thể mở rộng)
	if err := st.Outline.SaveLayeredOutline(fr.Volumes); err != nil {
		return fmt.Errorf("save layered outline: %w", err)
	}
	if err := st.Outline.SaveOutline(domain.FlattenOutline(fr.Volumes)); err != nil {
		return fmt.Errorf("save flattened outline: %w", err)
	}
	_ = st.Progress.UpdatePhase(domain.PhaseOutline)
	_ = st.Progress.SetTotalChapters(domain.TotalChapters(fr.Volumes))
	_ = st.Progress.SetLayered(true)
	if len(fr.Volumes) > 0 && len(fr.Volumes[0].Arcs) > 0 {
		_ = st.Progress.UpdateVolumeArc(fr.Volumes[0].Index, fr.Volumes[0].Arcs[0].Index)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "layered_outline", "layered_outline.json"); err != nil {
		return fmt.Errorf("checkpoint layered outline: %w", err)
	}

	// 5. la bàn (neo hướng tiếp tục): để layeredBookComplete được xác định dựa trên open_threads,
	//    Tránh phần giới thiệu có nghĩa là nó đã kết thúc; nó cũng đưa ra điểm chuẩn cho hướng/độ dài của phần tiếp theo.
	if err := st.Outline.SaveCompass(*fr.Compass); err != nil {
		return fmt.Errorf("save compass: %w", err)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "compass", "meta/compass.json"); err != nil {
		return fmt.Errorf("checkpoint compass: %w", err)
	}

	// 6. Nền tảng đã hoàn thiện → chuyển sang giai đoạn viết (phù hợp về mặt logic với phần cuối của save_foundation)
	if len(st.FoundationMissing()) == 0 {
		if p, _ := st.Progress.Load(); p != nil &&
			p.Phase != domain.PhaseWriting && p.Phase != domain.PhaseComplete {
			_ = st.Progress.UpdatePhase(domain.PhaseWriting)
		}
	}
	return nil
}

// giải mãJSON phân tích cú pháp JSON (mảng hoặc đối tượng) và đính kèm các thẻ để dễ dàng gỡ lỗi.
func decodeJSON(label, body string, out any) error {
	body = stripFences(body)
	if body == "" {
		return fmt.Errorf("%s body is empty", label)
	}
	if err := json.Unmarshal([]byte(body), out); err != nil {
		return fmt.Errorf("parse %s JSON: %w", label, err)
	}
	return nil
}

// StripFences loại bỏ hàng rào mã ``` đầu tiên và cuối cùng (bao gồm cả thẻ ngôn ngữ). LLM đôi khi sẽ bao gồm một lớp riêng.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if j := strings.LastIndex(s, "```"); j >= 0 {
		s = s[:j]
	}
	return strings.TrimSpace(s)
}
