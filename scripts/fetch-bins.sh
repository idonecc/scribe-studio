#!/usr/bin/env bash
#
# Fetch / build the native binaries Scribe ships with. Two modes:
#
#   --dev (default)    Symlink Homebrew's ffmpeg + whisper-cli into
#                      resources/bin/. Fast; good for `wails dev`.
#
#   --release          Drop Homebrew. Pull a static ffmpeg from
#                      evermeet.cx and build whisper-cli from source
#                      with BUILD_SHARED_LIBS=OFF. Output lives at
#                      resources/bin/{darwin-arm64,darwin-amd64}/ so
#                      build-release.sh can bundle them into the .app.
#
# Usage:
#   ./scripts/fetch-bins.sh             # dev mode (default)
#   ./scripts/fetch-bins.sh --release   # static binaries for shipping
#
# Why not pre-built whisper-cli? Upstream whisper.cpp only ships
# Windows + iOS xcframework binaries — no macOS pre-built. We build
# from source here.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

mode="${1:-dev}"

case "$mode" in
  --release|release)
    mode=release
    ;;
  --dev|dev|"")
    mode=dev
    ;;
  *)
    echo "[fetch-bins] unknown mode: $mode (use --dev or --release)" >&2
    exit 2
    ;;
esac

os="$(uname -s)"
if [[ "$os" != "Darwin" ]]; then
  echo "[fetch-bins] v0.2d only supports macOS. Skipping on $os." >&2
  exit 0
fi

arch="$(uname -m)"
case "$arch" in
  arm64)  archid="arm64"  ;;
  x86_64) archid="amd64"  ;;
  *)      echo "[fetch-bins] unsupported arch: $arch" >&2; exit 2 ;;
esac

if [[ "$mode" == "dev" ]]; then
  if ! command -v brew >/dev/null 2>&1; then
    echo "[fetch-bins] Homebrew is required for dev mode: https://brew.sh" >&2
    exit 1
  fi
  install_if_missing() {
    local formula="$1" exe="$2"
    if ! command -v "$exe" >/dev/null 2>&1; then
      echo "[fetch-bins] installing $formula..."
      brew install "$formula"
    fi
  }
  install_if_missing ffmpeg ffmpeg
  install_if_missing whisper-cpp whisper-cli
  install_if_missing yt-dlp yt-dlp

  mkdir -p resources/bin
  for tool in ffmpeg whisper-cli yt-dlp; do
    target="resources/bin/$tool"
    src="$(command -v "$tool")"
    if [[ -L "$target" && "$(readlink "$target")" == "$src" ]]; then
      continue
    fi
    rm -f "$target"
    ln -s "$src" "$target"
    echo "[fetch-bins] linked $target -> $src"
  done
  echo "[fetch-bins] dev setup done."
  exit 0
fi

# ---- release mode ---------------------------------------------------
# Place static binaries under resources/bin/darwin-{arch}/ so the Go
# runtime.BinaryPath resolver (that walks arch-specific subdirs) finds
# them inside the packaged .app.
bin_dir="resources/bin/darwin-${archid}"
mkdir -p "$bin_dir"

echo "[fetch-bins] release mode — target $bin_dir"

# ---- ffmpeg (evermeet static) --------------------------------------
if [[ -x "$bin_dir/ffmpeg" ]]; then
  echo "[fetch-bins] ffmpeg already in $bin_dir; skipping"
else
  workdir="$(mktemp -d)"
  trap 'rm -rf "$workdir"' EXIT
  zip="$workdir/ffmpeg.zip"
  # evermeet hosts both arm64 and x86 static builds; the same URL
  # serves whichever matches UA. We add arch to the query to be
  # explicit.
  echo "[fetch-bins] downloading static ffmpeg (universal build)"
  curl -fsSL -o "$zip" "https://evermeet.cx/ffmpeg/ffmpeg-7.1.zip"
  (cd "$workdir" && unzip -q ffmpeg.zip)
  install -m 0755 "$workdir/ffmpeg" "$bin_dir/ffmpeg"
  echo "[fetch-bins] ffmpeg -> $bin_dir/ffmpeg"
fi

# ---- whisper-cli (build from source, static) -----------------------
if [[ -x "$bin_dir/whisper-cli" ]]; then
  echo "[fetch-bins] whisper-cli already in $bin_dir; skipping"
else
  cache="${SCRIBE_BUILD_CACHE:-$HOME/.cache/scribe-build}"
  src="$cache/whisper.cpp"
  mkdir -p "$cache"
  if [[ ! -d "$src/.git" ]]; then
    echo "[fetch-bins] cloning whisper.cpp"
    git clone --depth 1 https://github.com/ggerganov/whisper.cpp.git "$src"
  else
    echo "[fetch-bins] refreshing whisper.cpp clone"
    (cd "$src" && git fetch --tags --depth 1 origin master && git reset --hard origin/master)
  fi

  echo "[fetch-bins] building whisper-cli (static libs + Metal)"
  # BUILD_SHARED_LIBS=OFF makes ggml static so the resulting binary
  # doesn't need libggml*.dylib alongside it at runtime.
  # WHISPER_METAL=1 gives GPU inference on Apple silicon.
  cmake -S "$src" -B "$src/build" \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=OFF \
    -DWHISPER_METAL=ON \
    -DGGML_METAL_EMBED_LIBRARY=ON \
    >/dev/null
  cmake --build "$src/build" --target whisper-cli --config Release -j
  install -m 0755 "$src/build/bin/whisper-cli" "$bin_dir/whisper-cli"
  echo "[fetch-bins] whisper-cli -> $bin_dir/whisper-cli"
fi

# ---- yt-dlp (official PyInstaller standalone) ----------------------
# yt-dlp ships a one-file macOS binary that already bundles a Python
# interpreter, so we can drop it next to ffmpeg without forcing users
# to install Python. The same binary works on both arm64 and x86_64.
if [[ -x "$bin_dir/yt-dlp" ]]; then
  echo "[fetch-bins] yt-dlp already in $bin_dir; skipping"
else
  echo "[fetch-bins] downloading yt-dlp (official macOS standalone)"
  curl -fsSL -o "$bin_dir/yt-dlp" \
    "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"
  chmod +x "$bin_dir/yt-dlp"
  echo "[fetch-bins] yt-dlp -> $bin_dir/yt-dlp"
fi

# ---- ad-hoc codesign so Gatekeeper accepts --------------------------
for tool in ffmpeg whisper-cli yt-dlp; do
  codesign --force --sign - "$bin_dir/$tool"
done

echo "[fetch-bins] release setup done."
echo "[fetch-bins] contents of $bin_dir:"
ls -la "$bin_dir"
