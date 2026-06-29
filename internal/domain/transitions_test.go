package domain

import "testing"

func TestCanTransitionPhase(t *testing.T) {
	tests := []struct {
		from Phase
		to   Phase
		want bool
	}{
		{from: "", to: PhaseInit, want: true},
		{from: PhaseInit, to: PhasePremise, want: true},
		{from: PhaseInit, to: PhaseOutline, want: true},
		{from: PhaseOutline, to: PhaseWriting, want: true},
		{from: PhaseWriting, to: PhaseComplete, want: true},
		{from: PhaseOutline, to: PhasePremise, want: false},
		{from: PhaseComplete, to: PhaseWriting, want: false},
	}
	for _, tt := range tests {
		if got := CanTransitionPhase(tt.from, tt.to); got != tt.want {
			t.Fatalf("CanTransitionPhase(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestCanTransitionFlow(t *testing.T) {
	tests := []struct {
		from FlowState
		to   FlowState
		want bool
	}{
		{from: "", to: FlowRewriting, want: true},
		{from: FlowWriting, to: FlowReviewing, want: true},
		{from: FlowReviewing, to: FlowPolishing, want: true},
		{from: FlowRewriting, to: FlowWriting, want: true},
		{from: FlowSteering, to: FlowRewriting, want: true},
		{from: FlowRewriting, to: FlowReviewing, want: false},
		{from: FlowPolishing, to: FlowReviewing, want: false},
	}
	for _, tt := range tests {
		if got := CanTransitionFlow(tt.from, tt.to); got != tt.want {
			t.Fatalf("CanTransitionFlow(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestExtractNovelNameFromPremise_Placeholder(t *testing.T) {
	cases := []struct {
		name    string
		premise string
		want    string
	}{
		{"tên sách thật", "# Đêm dài sẽ bình minh\n\n##", "Đêm dài sẽ bình minh"},
		{"Với tựa sách", "# \"Phía bên kia của thiên hà\" \n## Theme", "Phía bên kia của thiên hà"},
		{"Tiêu đề giữ chỗ của sách", "# Tên sách \n## Chủ đề", ""},
		{"Trình giữ chỗ - Tiêu đề sách mẫu", "# \"Tiêu đề sách mẫu\" \n## Chủ đề", ""},
		{"Giữ chỗ-tên sách thực tế", "# Tên sách thực tế \n## Chủ đề", ""},
		{"Dòng không có tiêu đề đầu tiên", "Dòng đầu tiên của văn bản thuần \n# tên sách", ""},
	}
	for _, c := range cases {
		if got := ExtractNovelNameFromPremise(c.premise); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
