package headless

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/tools"
)

func TestTerminalAskUserSingleSelect(t *testing.T) {
	handler := newTerminalAskUser(strings.NewReader("2\n"), &strings.Builder{})
	resp, err := handler.handle(context.Background(), []tools.Question{
		{
			Question: "Bạn muốn phong cách nào?",
			Header:   "phong cách",
			Options: []tools.Option{
				{Label: "say đắm", Description: "nâng cấp một phần"},
				{Label: "Hồi hộp", Description: "bí ẩn một phần"},
			},
		},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := resp.Answers["Bạn muốn phong cách nào?"]; got != "Hồi hộp" {
		t.Fatalf("unexpected answer: %q", got)
	}
}

func TestTerminalAskUserCustomInput(t *testing.T) {
	handler := newTerminalAskUser(strings.NewReader("0\n Không có đường tình yêu \n"), &strings.Builder{})
	resp, err := handler.handle(context.Background(), []tools.Question{
		{
			Question: "Có những hạn chế nào khác?",
			Header:   "giới hạn",
			Options: []tools.Option{
				{Label: "tối tăm", Description: "trầm cảm tổng thể"},
				{Label: "dễ", Description: "Tông màu tươi sáng"},
			},
		},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := resp.Answers["Có những hạn chế nào khác?"]; got != "Tùy chỉnh" {
		t.Fatalf("unexpected answer: %q", got)
	}
	if got := resp.Notes["Có những hạn chế nào khác?"]; got != "Không có đường tình yêu" {
		t.Fatalf("unexpected note: %q", got)
	}
}
