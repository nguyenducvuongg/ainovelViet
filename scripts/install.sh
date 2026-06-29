#!/bin/sh
# tập lệnh cài đặt bằng một cú nhấp chuột ainovel-cli
#
#   curl -fsSL https://raw.githubusercontent.com/nguyenducvuongg/ainovelViet/main/scripts/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/nguyenducvuongg/ainovelViet/main/scripts/install.sh | sh -s -- v1.2.3
#
# Thư mục cài đặt tùy chỉnh: AINOVEL_INSTALL_DIR=~/.local/bincurl -fsSL ... | sh
# Chỉ định phiên bản: AINOVEL_VERSION=v1.2.3curl -fsSL ... | sh
set -e

REPO="nguyenducvuongg/ainovelViet"
BIN="ainovel-cli"
DEST="${AINOVEL_INSTALL_DIR:-/usr/local/bin}"
VERSION="${AINOVEL_VERSION:-${1:-latest}}"

for cmd in curl tar; do
	lệnh -v "$cmd" >/dev/null 2>&1 || { echo "$cmd là bắt buộc, vui lòng cài đặt nó trước và thử lại"; lối ra 1; }
done

case "$(uname -s)" in
	Darwin) OS="Darwin" ;;
	Linux)  OS="Linux" ;;
	*) echo "Hệ thống không được hỗ trợ $(uname -s); Windows vui lòng truy cập https://github.com/$REPO/releases để tải xuống thủ công"; lối ra 1;;
esac

case "$(uname -m)" in
	x86_64|amd64)  ARCH="x86_64" ;;
	arm64|aarch64) ARCH="arm64" ;;
	*) echo "Kiến trúc không được hỗ trợ $(uname -m)"; lối ra 1 ;;
esac

if [ "$VERSION" = "latest" ] || [ -z "$VERSION" ]; then
	API="https://api.github.com/repos/$REPO/releases/latest"
	echo "Truy vấn phiên bản mới nhất..."
else
	case "$VERSION" in
		v*) TAG="$VERSION" ;;
		*) TAG="v$VERSION" ;;
	esac
	API="https://api.github.com/repos/$REPO/releases/tags/$TAG"
	echo "Phiên bản truy vấn $TAG..."
fi

RELEASE=$(curl -fsSL "$API")
TAG=$(printf '%s\n' "$RELEASE" | grep '"tag_name"' | head -1 | cut -d '"' -f 4)
URL=$(printf '%s\n' "$RELEASE" \
	| grep "browser_download_url" \
	| grep "_${OS}_${ARCH}.tar.gz" \
	| head -1 | cut -d '"' -f 4)
[ -n "$URL" ] || { echo "Không tìm thấy gói cài đặt ${OS__${ARCH}, vui lòng truy cập https://github.com/$REPO/releases để tải xuống thủ công"; lối ra 1; }

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Tải xuống $URL"
curl -fsSL -o "$TMP/pkg.tar.gz" "$URL"
tar -xzf "$TMP/pkg.tar.gz" -C "$TMP"

echo "Cài đặt vào $DEST"
[ -d "$DEST" ] || mkdir -p "$DEST" 2>/dev/null || sudo mkdir -p "$DEST"
if [ -w "$DEST" ]; then
	mv "$TMP/$BIN" "$DEST/$BIN"
else
	echo "Cần có đặc quyền của quản trị viên để ghi vào $DEST"
	sudo mv "$TMP/$BIN" "$DEST/$BIN"
fi
chmod +x "$DEST/$BIN"

# Tệp nhị phân không được ký. Lần chạy macOS đầu tiên sẽ bị Gatekeeper chặn và giải phóng sự cô lập.
[ "$OS" = "Darwin" ] && xattr -d com.apple.quarantine "$DEST/$BIN" 2>/dev/null || true

echo " ✓ Hoàn tất cài đặt: $DEST/$BIN"
[ -n "$TAG" ] && echo "Phiên bản: $TAG"
lệnh -v "$BIN" >/dev/null 2>&1 || echo "Nhắc: $DEST không có trong PATH, vui lòng thêm nó vào PATH"
echo "Chạy $BIN để bắt đầu"
