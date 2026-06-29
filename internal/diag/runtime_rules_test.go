package diag

import "testing"

// TestRuntimeFindings_Classify chứng minh rằng chữ ký trùng lặp được phân loại chính xác theo biểu mẫu và ngưỡng được nâng cấp hoặc hạ cấp chính xác.
// Và khi chạy thì Tìm tất cả AutoNone (kỷ luật người quan sát: chỉ chẩn đoán chứ không đưa ra hành động).
func TestRuntimeFindings_Classify(t *testing.T) {
	rc := RuntimeCapture{
		Repeats: []RepeatStat{
			{Sig: "coordinator · err: InputValidationError", Count: 14}, // vòng lặp lỗi nghiêm trọng
			{Sig: "coordinator · subagent", Count: 45},                  // Dụng cụ tần số cao thông thường → Không được sản xuất Đang tìm kiếm
			{Sig: "writer · save_plan (args invalid)", Count: 4},        // Cảnh báo tham số không hợp lệ
		},
		StuckStep:  "writing.commit_ch07",
		StuckCount: 9, // bị mắc kẹt quan trọng
		LogKinds:   map[string]int{"stream_idle": 4},
		LogErrors:  270, // Chạy đường dài tích lũy và không nên sản xuất một mình
	}

	fs := runtimeFindings(&rc)
	sev := map[string]Severity{}
	for _, f := range fs {
		sev[f.Rule] = f.Severity
		if f.AutoLevel != AutoNone {
			t.Errorf("%s phải là AutoNone (kỷ luật người quan sát), có %s", f.Rule, f.AutoLevel)
		}
	}

	want := map[string]Severity{
		"RepeatedToolError": SevCritical,
		"ArgsInvalidLoop":   SevWarning,
		"StuckStep":         SevCritical,
		"StreamIdleStorm":   SevWarning,
	}
	for rule, w := range want {
		if sev[rule] != w {
			t.Errorf("%s: got %q want %q", rule, sev[rule], w)
		}
	}
	// Lỗi tích lũy nhật ký/công cụ tần số cao thông thường sẽ không tạo ra Tìm kiếm (để tránh cảnh báo sai trong thời gian dài).
	if _, ok := sev["RepeatedToolCall"]; ok {
		t.Error("Việc sao chép công cụ thông thường sẽ không tạo ra Tìm kiếm")
	}
	if _, ok := sev["LogErrorBurst"]; ok {
		t.Error("Không nên tạo tích lũy lỗi nhật ký riêng lẻ")
	}
}

// TestRuntimeFindings_Quiet chứng minh rằng không có Kết quả trong thời gian chạy nào được tạo ra (không có kết quả dương tính giả) khi không có tín hiệu ngoại lệ.
func TestRuntimeFindings_Quiet(t *testing.T) {
	rc := RuntimeCapture{
		LogKinds:  map[string]int{"stream_idle": 1}, // dưới ngưỡng
		LogErrors: 2,
	}
	if fs := runtimeFindings(&rc); len(fs) != 0 {
		t.Errorf("Không nên tạo tĩnh ổn định Đang tìm, có %d: %+v", len(fs), fs)
	}
}
