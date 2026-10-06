#!/bin/sh
# Downloads the pinned Vosk speech model used for voice dictation in the AI
# panel composer (recognition runs entirely in the webview via
# vosk-browser — see frontend/src/lib/dictation.ts). Lands in
# frontend/public/vosk/, which vite copies into dist so the model ships
# inside the app. Run by `make init`/`make dev`/`make build`/`make darwin`.
# Safe to skip offline — the mic button just reports the model as missing.
set -e

NAME=vosk-model-small-en-us-0.15
SHA256=f0b24bb92a48ca575b6a96500d6b543f0f079c573dfe85bbe16001fc0404e1d8
OUT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)/frontend/public/vosk"
dest="$OUT/model.tar.gz"

if [ -f "$dest" ] && [ -z "$FORCE" ]; then
  echo "vosk: model already present, skip"
  exit 0
fi
mkdir -p "$OUT"
echo "vosk: fetching $NAME"
# vosk-browser needs a .tar.gz of the model folder; upstream alphacephei only
# publishes .zip, so this pulls vosk-browser's own repackaged copy.
curl -fL --progress-bar -o "$dest.part" "https://ccoreilly.github.io/vosk-browser/models/$NAME.tar.gz"
echo "$SHA256  $dest.part" | shasum -a 256 -c - >/dev/null
mv "$dest.part" "$dest"
