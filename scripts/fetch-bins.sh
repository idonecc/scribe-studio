#!/usr/bin/env bash
#
# Fetch the native binaries Scribe shells out to. For v0.2a we lean on
# Homebrew on macOS — keeping distribution concerns (static builds,
# dylib rpaths, code signing) out of this stage. The production
# release pipeline (v0.2d) will graduate to pre-built static binaries
# placed directly in resources/bin/.
#
# Usage:
#   ./scripts/fetch-bins.sh
#
# What it does:
#   1. Ensures `ffmpeg` is present (brew install if missing).
#   2. Ensures `whisper-cli` is present (brew install whisper-cpp).
#   3. Symlinks both into resources/bin/ so a dev running `wails dev`
#      gets the same resolution path as a bundled build will later.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

os="$(uname -s)"
if [[ "$os" != "Darwin" ]]; then
  echo "[fetch-bins] v0.2a only supports macOS via Homebrew. Skipping on $os." >&2
  exit 0
fi

if ! command -v brew >/dev/null 2>&1; then
  echo "[fetch-bins] Homebrew is required: https://brew.sh" >&2
  exit 1
fi

install_if_missing() {
  local formula="$1" exe="$2"
  if command -v "$exe" >/dev/null 2>&1; then
    return
  fi
  echo "[fetch-bins] installing $formula..."
  brew install "$formula"
}

install_if_missing ffmpeg ffmpeg
install_if_missing whisper-cpp whisper-cli

mkdir -p resources/bin
for tool in ffmpeg whisper-cli; do
  target="resources/bin/$tool"
  src="$(command -v "$tool")"
  if [[ -L "$target" && "$(readlink "$target")" == "$src" ]]; then
    continue
  fi
  rm -f "$target"
  ln -s "$src" "$target"
  echo "[fetch-bins] linked $target -> $src"
done

echo "[fetch-bins] done."
