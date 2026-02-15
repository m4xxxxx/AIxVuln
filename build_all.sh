#!/usr/bin/env bash
#
# AIxVuln 一键交叉编译脚本
#
# 产出目录: dist/
#   dist/AIxVuln-web-<os>-<arch>[.exe]       — WEB 版 (纯 HTTP 服务, 无桌面窗口)
#   dist/AIxVuln-gui-<os>-<arch>[.exe/.app]  — GUI 版 (Wails 桌面应用)
#
# 用法:
#   ./build_all.sh              # 构建全部
#   ./build_all.sh web          # 仅构建 WEB 版 (全平台, 无需 CGO)
#   ./build_all.sh gui          # 仅构建 GUI 版 (需要 CGO + 交叉编译工具链)
#   ./build_all.sh web linux    # 仅构建 WEB 版 Linux 目标
#
# WEB 版交叉编译 (CGO_ENABLED=0, 纯 Go):
#   ✅ macOS   amd64 / arm64
#   ✅ Linux   amd64 / arm64
#   ✅ Windows amd64 / arm64
#
# GUI 版交叉编译 (CGO_ENABLED=1, 需要平台工具链):
#   ✅ macOS   amd64 / arm64  — 需要 Xcode CommandLineTools (本机 macOS)
#   ⚠️ Windows amd64          — 需要 mingw-w64 (brew install mingw-w64)
#   ⚠️ Windows arm64          — 需要 llvm-mingw (见下方说明)
#   ❌ Linux   amd64 / arm64  — Wails 依赖 GTK3+WebKitGTK, 建议在 Linux 上原生构建
#
# GUI Linux 构建说明:
#   在 Linux 机器上安装依赖后运行:
#     sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev
#     ./build_all.sh gui linux
#
# Windows mingw-w64 安装 (macOS):
#   brew install mingw-w64
#
# Windows arm64 需要 llvm-mingw:
#   brew install llvm-mingw   # 或从 https://github.com/mstorsjo/llvm-mingw 下载
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$SCRIPT_DIR"
GUI_DIR="$PROJECT_DIR/wailsapp/AIxVulnGUI"
FRONTEND_DIR="$GUI_DIR/frontend"
DIST_DIR="$PROJECT_DIR/dist"

BUILD_MODE="${1:-all}"   # all | web | gui
OS_FILTER="${2:-}"       # 可选: darwin | linux | windows

# ─── 颜色输出 ───
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }
skip()  { echo -e "${YELLOW}[SKIP]${NC}  $*"; }

# ─── 前置检查 ───
check_tool() {
    if ! command -v "$1" &>/dev/null; then
        fail "需要 $1，请先安装: $2"
        return 1
    fi
}

check_tool go   "https://go.dev/dl/"
check_tool node "https://nodejs.org/"
check_tool npm  "https://nodejs.org/"

# ─── 准备工作 ───
mkdir -p "$DIST_DIR"

# 1) 构建前端 dist
info "构建前端..."
(cd "$FRONTEND_DIR" && npm install --silent && npm run build)
ok "前端构建完成"

# 2) 复制 dockerfile 目录 (go:embed 不支持符号链接)
NEED_CLEANUP_DOCKERFILE=false
if [ ! -d "$GUI_DIR/dockerfile" ]; then
    info "复制 dockerfile 目录到构建目录..."
    cp -R "$PROJECT_DIR/dockerfile" "$GUI_DIR/dockerfile"
    NEED_CLEANUP_DOCKERFILE=true
    ok "dockerfile 已复制"
fi

cleanup() {
    if $NEED_CLEANUP_DOCKERFILE && [ -d "$GUI_DIR/dockerfile" ]; then
        rm -rf "$GUI_DIR/dockerfile"
        info "已清理临时 dockerfile 目录"
    fi
}
trap cleanup EXIT

# ─── 构建函数 ───

# build_web <GOOS> <GOARCH>
build_web() {
    local os="$1" arch="$2"
    local ext=""; [ "$os" = "windows" ] && ext=".exe"
    local out="$DIST_DIR/AIxVuln-web-${os}-${arch}${ext}"

    info "WEB  ${os}/${arch} ..."
    (cd "$GUI_DIR" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -tags '' \
        -ldflags="-s -w" \
        -o "$out" \
        .)
    ok "WEB  ${os}/${arch} -> $(basename "$out")"
}

# build_gui <GOOS> <GOARCH> [CC]
build_gui() {
    local os="$1" arch="$2" cc="${3:-}"
    local ext=""; [ "$os" = "windows" ] && ext=".exe"
    local out="$DIST_DIR/AIxVuln-gui-${os}-${arch}${ext}"

    if [ -n "$cc" ] && ! command -v "$cc" &>/dev/null; then
        skip "GUI  ${os}/${arch} — 缺少交叉编译器 $cc"
        return 0
    fi

    info "GUI  ${os}/${arch} (CC=${cc:-default}) ..."

    (
        cd "$GUI_DIR"
        export CGO_ENABLED=1 GOOS="$os" GOARCH="$arch"
        [ -n "$cc" ] && export CC="$cc"
        # macOS 需要链接 UniformTypeIdentifiers framework (Wails 依赖)
        [ "$os" = "darwin" ] && export CGO_LDFLAGS="-framework UniformTypeIdentifiers"
        go build -tags desktop,production \
            -ldflags="-s -w" \
            -o "$out" \
            .
    )
    ok "GUI  ${os}/${arch} -> $(basename "$out")"
}

# ─── 目标矩阵 ───

should_build() {
    [ -z "$OS_FILTER" ] || [ "$OS_FILTER" = "$1" ]
}

build_all_web() {
    info "========== WEB 版构建 (CGO_ENABLED=0) =========="
    should_build darwin  && build_web darwin  amd64
    should_build darwin  && build_web darwin  arm64
    should_build linux   && build_web linux   amd64
    should_build linux   && build_web linux   arm64
    should_build windows && build_web windows amd64
    should_build windows && build_web windows arm64
}

build_all_gui() {
    info "========== GUI 版构建 (CGO_ENABLED=1) =========="

    local host_os host_arch
    # 用 uname 检测真实宿主机 OS/ARCH，而非 go env（后者可能被 go env -w 覆盖）
    case "$(uname -s)" in
        Darwin*) host_os="darwin" ;;
        Linux*)  host_os="linux" ;;
        MINGW*|MSYS*|CYGWIN*) host_os="windows" ;;
        *)       host_os="unknown" ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) host_arch="amd64" ;;
        arm64|aarch64) host_arch="arm64" ;;
        *)            host_arch="unknown" ;;
    esac

    # macOS — 本机 + 交叉 (arm64 <-> amd64 通过 Apple Clang 支持)
    if should_build darwin; then
        if [ "$host_os" = "darwin" ]; then
            build_gui darwin amd64
            build_gui darwin arm64
        else
            skip "GUI  darwin/* — 需要在 macOS 上构建"
        fi
    fi

    # Windows — 通过 mingw-w64 交叉编译
    if should_build windows; then
        build_gui windows amd64 x86_64-w64-mingw32-gcc
        build_gui windows arm64 aarch64-w64-mingw32-gcc
    fi

    # Linux — 需要 GTK3 + WebKitGTK 开发库
    if should_build linux; then
        if [ "$host_os" = "linux" ]; then
            # 本机架构直接编译
            build_gui linux "$host_arch"
            # 交叉架构需要对应的 gcc
            if [ "$host_arch" = "amd64" ]; then
                build_gui linux arm64 aarch64-linux-gnu-gcc
            elif [ "$host_arch" = "arm64" ]; then
                build_gui linux amd64 x86_64-linux-gnu-gcc
            fi
        else
            skip "GUI  linux/* — Wails 依赖 GTK3+WebKitGTK, 建议在 Linux 上原生构建"
        fi
    fi
}

# ─── 执行 ───

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║       AIxVuln 交叉编译构建                   ║"
echo "╠══════════════════════════════════════════════╣"
echo "║  模式: $(printf '%-37s' "$BUILD_MODE") ║"
echo "║  平台: $(printf '%-37s' "${OS_FILTER:-全部}") ║"
echo "╚══════════════════════════════════════════════╝"
echo ""

case "$BUILD_MODE" in
    web)
        build_all_web
        ;;
    gui)
        build_all_gui
        ;;
    all)
        build_all_web
        echo ""
        build_all_gui
        ;;
    *)
        fail "未知模式: $BUILD_MODE (可选: all, web, gui)"
        exit 1
        ;;
esac

echo ""
info "========== 构建产出 =========="
ls -lh "$DIST_DIR"/AIxVuln-* 2>/dev/null || warn "没有构建产出"
echo ""
ok "构建完成！产出目录: $DIST_DIR/"
