#!/usr/bin/env bash
# Draws the app's icons in public/ from the brand's mark (the BrandMark
# component's, in its light colours): icon.svg, rounded, for a browser's
# tab and the installed app; maskable.svg, full bleed with the curve
# inside the safe zone, for a launcher that masks it and for iOS, which
# rounds its own corners. Run it again, with rsvg-convert, when the mark
# changes; the PNGs are committed.
set -euo pipefail
cd "$(dirname "$0")/.."

rsvg-convert -w 192 -h 192 public/icon.svg -o public/pwa-192.png
rsvg-convert -w 512 -h 512 public/icon.svg -o public/pwa-512.png
rsvg-convert -w 512 -h 512 icons/maskable.svg -o public/maskable-512.png
rsvg-convert -w 180 -h 180 icons/maskable.svg -o public/apple-touch-icon.png
