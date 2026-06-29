package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/voocel/ainovel-cli/internal/tools"
)

//go:embed prompts/*.md
var promptsFS embed.FS

//go:embed references
var referencesFS embed.FS

//go:embed styles/*.md
var stylesFS embed.FS

//go:embed rules
var rulesFS embed.FS

// Lời nhắc đại diện cho một tập hợp các từ nhắc nhở được nhúng.
type Prompts struct {
	Coordinator      string
	ArchitectShort   string
	ArchitectLong    string
	Writer           string
	Editor           string
	ImportFoundation string
	ImportAnalyzer   string
	SimulationSource string
	SimulationMerge  string
}

// Bundle đại diện cho một tập hợp các tài nguyên tĩnh cần thiết cho hoạt động.
type Bundle struct {
	References tools.References
	Prompts    Prompts
	Styles     map[string]string
	// RulesFS là cây con nội dung/quy tắc (thư mục gốc chứa trực tiếp default.md).
	// Người gọi chuyển các quy tắc.Load làm nguồn của các quy tắc tích hợp.
	RulesFS fs.FS
}

// Tải trả về bộ sưu tập tài nguyên tương ứng với kiểu đã chỉ định.
func Load(style string) Bundle {
	return Bundle{
		References: loadReferences(style),
		Prompts:    loadPrompts(),
		Styles:     loadStyles(),
		RulesFS:    loadRulesFS(),
	}
}

// LoadRulesFS trả về hệ thống tệp con của nội dung/quy tắc; thư mục gốc chứa trực tiếp default.md.
// Trả về 0 khi fs.Sub bị lỗi (điều này về mặt lý thuyết là không nên xảy ra) và Rules.Load bỏ qua nguồn tích hợp tương ứng.
func loadRulesFS() fs.FS {
	sub, err := fs.Sub(rulesFS, "rules")
	if err != nil {
		return nil
	}
	return sub
}

func loadReferences(style string) tools.References {
	if style == "" {
		style = "default"
	}
	refs := tools.References{
		ChapterGuide:      mustRead(referencesFS, "references/chapter-guide.md"),
		HookTechniques:    mustRead(referencesFS, "references/hook-techniques.md"),
		QualityChecklist:  mustRead(referencesFS, "references/quality-checklist.md"),
		OutlineTemplate:   mustRead(referencesFS, "references/outline-template.md"),
		CharacterTemplate: mustRead(referencesFS, "references/character-template.md"),
		ChapterTemplate:   mustRead(referencesFS, "references/chapter-template.md"),
		Consistency:       mustRead(referencesFS, "references/consistency.md"),
		ContentExpansion:  mustRead(referencesFS, "references/content-expansion.md"),
		DialogueWriting:   mustRead(referencesFS, "references/dialogue-writing.md"),
		LongformPlanning:  mustRead(referencesFS, "references/longform-planning.md"),
		Differentiation:   mustRead(referencesFS, "references/differentiation.md"),
		AntiAITone:        mustRead(referencesFS, "references/anti-ai-tone.md"),
	}
	if style != "" && style != "default" {
		genreDir := "references/genres/" + style + "/"
		if data, err := referencesFS.ReadFile(genreDir + "style-references.md"); err == nil {
			refs.StyleReference = string(data)
		}
		if data, err := referencesFS.ReadFile(genreDir + "arc-templates.md"); err == nil {
			refs.ArcTemplates = string(data)
		}
	}
	return refs
}

func loadPrompts() Prompts {
	return Prompts{
		Coordinator:      withSimulationGuidance(mustRead(promptsFS, "prompts/coordinator.md"), "coordinator"),
		ArchitectShort:   withSimulationGuidance(mustRead(promptsFS, "prompts/architect-short.md"), "architect"),
		ArchitectLong:    withSimulationGuidance(mustRead(promptsFS, "prompts/architect-long.md"), "architect"),
		Writer:           withSimulationGuidance(mustRead(promptsFS, "prompts/writer.md"), "writer"),
		Editor:           withSimulationGuidance(mustRead(promptsFS, "prompts/editor.md"), "editor"),
		ImportFoundation: mustRead(promptsFS, "prompts/import-foundation.md"),
		ImportAnalyzer:   mustRead(promptsFS, "prompts/import-chapter-analyzer.md"),
		SimulationSource: mustRead(promptsFS, "prompts/simulation-source.md"),
		SimulationMerge:  mustRead(promptsFS, "prompts/simulation-merge.md"),
	}
}

func withSimulationGuidance(prompt, role string) string {
	return prompt + "\n\n" + strings.ReplaceAll(simulationGuidance, "{{role}}", role)
}

const simulationGuidance = `## Chân dung mô phỏng

Khi Novel_context trả về một mô phỏng_profile, nó phải được coi là một ràng buộc về hướng mô phỏng cho tác phẩm hiện tại. {{role}} nên đọc kiểu, từ vựng, cốt truyện, hook_design, pacing_dense, reader_engagement và role_guidance.

Nguyên tắc sử dụng: rút kinh nghiệm từ cấu trúc, nhịp điệu, câu móc, truyền tải thông tin và thu hút người đọc; không sao chép câu gốc, ký tự, địa danh, cài đặt độc quyền hoặc phần cố định. Nếu mô phỏng_profile xung đột với yêu cầu rõ ràng của người dùng thì yêu cầu của người dùng sẽ được ưu tiên. `

func loadStyles() map[string]string {
	styles := make(map[string]string)
	entries, err := stylesFS.ReadDir("styles")
	if err != nil {
		return styles
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		data, err := stylesFS.ReadFile("styles/" + e.Name())
		if err != nil {
			continue
		}
		styles[name] = string(data)
	}
	return styles
}

func mustRead(fs embed.FS, path string) string {
	data, err := fs.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("embed read %s: %v", path, err))
	}
	return string(data)
}
