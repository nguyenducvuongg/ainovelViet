#!/bin/sh
#
# Generate AI-summarized release notes from git commits.
# Usage: .github/scripts/gen-changelog.sh [previous_tag]
#
# Requires GEMINI_API_KEY (preferred), ANTHROPIC_API_KEY, or OPENAI_API_KEY.
# Falls back to raw commit list if no API key is set.
#
set -e

PREV_TAG="${1:-$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || echo "")}"
CURR_TAG="$(git describe --tags --abbrev=0 HEAD 2>/dev/null || echo "HEAD")"

if [ -n "$PREV_TAG" ]; then
    COMMITS=$(git log "${PREV_TAG}..${CURR_TAG}" --pretty=format:"- %s" --no-merges)
    RANGE="${PREV_TAG}..${CURR_TAG}"
else
    COMMITS=$(git log --pretty=format:"- %s" --no-merges -50)
    RANGE="last 50 commits"
fi

if [ -z "$COMMITS" ]; then
    echo "No commits found in range ${RANGE}"
    exit 0
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

cat > "$TMPDIR/prompt.txt" <<PROMPT_EOF
Bạn là tác giả của ghi chú phát hành cho công cụ dòng lệnh Go ainovel-cli, một công cụ viết tiểu thuyết AI.
Vui lòng tạo hướng dẫn phát hành Markdown tiếng Trung ngắn gọn, rõ ràng, hướng đến người dùng dựa trên bản ghi cam kết Git bên dưới.

luật lệ:
- Sử dụng đầu ra tiếng Trung
- Sắp xếp nội dung thành các nhóm sau: tính năng mới, sửa lỗi, tối ưu hóa hiệu suất, tái cấu trúc, các nhóm khác; không xuất các nhóm không có nội dung
- Mỗi dòng một dòng, giữ đơn giản, không bao gồm hàm băm xác nhận hoặc tên tác giả
- Loại bỏ các tiền tố commit thông thường như feat:, fix:, perf:, refactor:, v.v.
- Hợp nhất các bài gửi tương tự hoặc trùng lặp để tránh lặp lại một cách máy móc các cam kết từng cái một
- Sử dụng các biểu thức hướng đến người dùng để làm nổi bật những thay đổi và tác động thực tế
- Tập trung vào những thay đổi mà người dùng có thể nhận biết được như quy trình phát hành, đóng gói nhị phân, hành vi CLI/TUI, quy trình soạn thảo, hỗ trợ mô hình và tài liệu
- Chỉ xuất nội dung Markdown, không xuất lời mở đầu, giải thích hay tóm tắt

Cam kết hồ sơ (${RANGE}):
${COMMITS}
PROMPT_EOF

# Build JSON body with jq (reads from file to handle special chars).
build_body() { jq -Rs "$1" < "$TMPDIR/prompt.txt" > "$TMPDIR/body.json"; }

# Extract text from JSON response (python3 handles control chars reliably).
extract() { python3 -c "import json,sys; d=json.load(open('$TMPDIR/result.json')); print($1)"; }

fallback() {
    echo "## What's Changed"
    echo ""
    echo "$COMMITS"
}

# Try Gemini first, then Anthropic, then OpenAI.
if [ -n "$GEMINI_API_KEY" ]; then
    API_URL="${GEMINI_BASE_URL:-https://generativelanguage.googleapis.com}/v1beta/models/gemini-2.5-flash:generateContent?key=${GEMINI_API_KEY}"
    build_body '{contents: [{parts: [{text: .}]}]}'
    if curl -fsSL "$API_URL" -H "content-type: application/json" -d @"$TMPDIR/body.json" -o "$TMPDIR/result.json"; then
        extract "d['candidates'][0]['content']['parts'][0]['text']"
    else
        fallback
    fi

elif [ -n "$ANTHROPIC_API_KEY" ]; then
    API_URL="${ANTHROPIC_BASE_URL:-https://api.anthropic.com}/v1/messages"
    build_body '{model: "claude-sonnet-4-5-20250514", max_tokens: 1024, messages: [{role: "user", content: .}]}'
    if curl -fsSL "$API_URL" -H "x-api-key: ${ANTHROPIC_API_KEY}" -H "anthropic-version: 2023-06-01" -H "content-type: application/json" -d @"$TMPDIR/body.json" -o "$TMPDIR/result.json"; then
        extract "d['content'][0]['text']"
    else
        fallback
    fi

elif [ -n "$OPENAI_API_KEY" ]; then
    API_URL="${OPENAI_BASE_URL:-https://api.openai.com}/v1/chat/completions"
    build_body '{model: "gpt-4o-mini", messages: [{role: "user", content: .}]}'
    if curl -fsSL "$API_URL" -H "Authorization: Bearer ${OPENAI_API_KEY}" -H "content-type: application/json" -d @"$TMPDIR/body.json" -o "$TMPDIR/result.json"; then
        extract "d['choices'][0]['message']['content']"
    else
        fallback
    fi

else
    fallback
fi
