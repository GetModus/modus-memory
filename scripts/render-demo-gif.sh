#!/usr/bin/env bash

set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This renderer currently expects macOS Quick Look (qlmanage)." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
RENDER_DIR="${TMP_DIR}/rendered"
mkdir -p "${RENDER_DIR}"
trap 'rm -rf "${TMP_DIR}"' EXIT

write_scene_01() {
  cat >"${TMP_DIR}/scene-01.svg" <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="675" viewBox="0 0 1200 675">
  <rect width="1200" height="675" fill="#0b1020"/>
  <rect x="36" y="28" width="1128" height="619" rx="18" fill="#121a2b" stroke="#263247" stroke-width="2"/>
  <rect x="36" y="28" width="1128" height="46" rx="18" fill="#161b22"/>
  <rect x="36" y="56" width="1128" height="18" fill="#161b22"/>
  <circle cx="70" cy="51" r="6" fill="#ff5f57"/>
  <circle cx="92" cy="51" r="6" fill="#febb2e"/>
  <circle cx="114" cy="51" r="6" fill="#28c840"/>
  <text x="530" y="56" font-family="system-ui, -apple-system, sans-serif" font-size="20" fill="#e6edf3" text-anchor="middle">Homing by MODUS</text>
  <text x="953" y="56" font-family="monospace" font-size="14" fill="#7d8590" text-anchor="middle">remember • recall • attach</text>

  <rect x="74" y="108" width="194" height="34" rx="17" fill="#13213a" stroke="#2f81f7" stroke-width="2"/>
  <text x="171" y="131" font-family="system-ui, -apple-system, sans-serif" font-size="22" fill="#58a6ff" text-anchor="middle">1. Remember</text>
  <text x="302" y="131" font-family="system-ui, -apple-system, sans-serif" font-size="20" fill="#e6edf3">Natural-language capture becomes local memory.</text>

  <rect x="94" y="182" width="1000" height="122" rx="16" fill="#111b30" stroke="#1f6feb" stroke-width="2"/>
  <text x="122" y="215" font-family="monospace" font-size="26" fill="#58a6ff">You:</text>
  <text x="122" y="251" font-family="monospace" font-size="24" fill="#e6edf3">Remember that we chose Postgres for billing</text>
  <text x="122" y="286" font-family="monospace" font-size="24" fill="#e6edf3">because JSONB support mattered.</text>

  <rect x="94" y="332" width="1000" height="138" rx="16" fill="#12271d" stroke="#3fb950" stroke-width="2"/>
  <text x="122" y="366" font-family="monospace" font-size="25" fill="#7ee787">Homing:</text>
  <text x="122" y="401" font-family="monospace" font-size="22" fill="#e6edf3">Stored as durable memory with route cues:</text>
  <text x="122" y="436" font-family="monospace" font-size="22" fill="#e6edf3">billing, postgres, jsonb, database-choice</text>

  <rect x="94" y="498" width="1000" height="54" rx="12" fill="#0f172a" stroke="#7d8590" stroke-width="1"/>
  <text x="122" y="530" font-family="monospace" font-size="18" fill="#8b949e">memory/facts/billing-service-database-choice.md</text>
  <text x="707" y="530" font-family="monospace" font-size="18" fill="#8b949e">receipt: recall-2026-04-16-001.md</text>

  <rect x="74" y="578" width="1050" height="40" rx="12" fill="#111827" stroke="#30363d" stroke-width="2"/>
  <text x="599" y="603" font-family="monospace" font-size="16" fill="#8b949e" text-anchor="middle">sovereign memory kernel • plain markdown • one binary</text>
</svg>
EOF
}

write_scene_02() {
  cat >"${TMP_DIR}/scene-02.svg" <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="675" viewBox="0 0 1200 675">
  <rect width="1200" height="675" fill="#0b1020"/>
  <rect x="36" y="28" width="1128" height="619" rx="18" fill="#121a2b" stroke="#263247" stroke-width="2"/>
  <rect x="36" y="28" width="1128" height="46" rx="18" fill="#161b22"/>
  <rect x="36" y="56" width="1128" height="18" fill="#161b22"/>
  <circle cx="70" cy="51" r="6" fill="#ff5f57"/>
  <circle cx="92" cy="51" r="6" fill="#febb2e"/>
  <circle cx="114" cy="51" r="6" fill="#28c840"/>
  <text x="530" y="56" font-family="system-ui, -apple-system, sans-serif" font-size="20" fill="#e6edf3" text-anchor="middle">Homing by MODUS</text>
  <text x="953" y="56" font-family="monospace" font-size="14" fill="#7d8590" text-anchor="middle">remember • recall • attach</text>

  <rect x="74" y="108" width="160" height="34" rx="17" fill="#13213a" stroke="#2f81f7" stroke-width="2"/>
  <text x="154" y="131" font-family="system-ui, -apple-system, sans-serif" font-size="22" fill="#58a6ff" text-anchor="middle">2. Recall</text>
  <text x="262" y="131" font-family="system-ui, -apple-system, sans-serif" font-size="20" fill="#e6edf3">Route-aware retrieval narrows before ranking.</text>

  <rect x="94" y="184" width="1000" height="92" rx="16" fill="#111b30" stroke="#1f6feb" stroke-width="2"/>
  <text x="122" y="218" font-family="monospace" font-size="26" fill="#58a6ff">You:</text>
  <text x="122" y="254" font-family="monospace" font-size="24" fill="#e6edf3">What database should we use for payments?</text>

  <rect x="94" y="302" width="1000" height="168" rx="16" fill="#12271d" stroke="#3fb950" stroke-width="2"/>
  <text x="122" y="336" font-family="monospace" font-size="25" fill="#7ee787">Homing:</text>
  <text x="122" y="371" font-family="monospace" font-size="22" fill="#e6edf3">Recalled the billing decision through related-service cues.</text>
  <text x="122" y="406" font-family="monospace" font-size="22" fill="#e6edf3">Recommendation: Postgres is the consistent choice.</text>
  <text x="122" y="441" font-family="monospace" font-size="22" fill="#e6edf3">JSONB still fits flexible payment metadata.</text>

  <rect x="94" y="496" width="1000" height="54" rx="12" fill="#0f172a" stroke="#7d8590" stroke-width="1"/>
  <text x="122" y="530" font-family="monospace" font-size="18" fill="#8b949e">route: service-family → billing → database-decision</text>
  <text x="734" y="530" font-family="monospace" font-size="18" fill="#8b949e">sources: 3 linked artifacts</text>

  <rect x="74" y="578" width="1050" height="40" rx="12" fill="#111827" stroke="#30363d" stroke-width="2"/>
  <text x="599" y="603" font-family="monospace" font-size="16" fill="#8b949e" text-anchor="middle">sovereign memory kernel • plain markdown • one binary</text>
</svg>
EOF
}

write_scene_03() {
  cat >"${TMP_DIR}/scene-03.svg" <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="675" viewBox="0 0 1200 675">
  <rect width="1200" height="675" fill="#0b1020"/>
  <rect x="36" y="28" width="1128" height="619" rx="18" fill="#121a2b" stroke="#263247" stroke-width="2"/>
  <rect x="36" y="28" width="1128" height="46" rx="18" fill="#161b22"/>
  <rect x="36" y="56" width="1128" height="18" fill="#161b22"/>
  <circle cx="70" cy="51" r="6" fill="#ff5f57"/>
  <circle cx="92" cy="51" r="6" fill="#febb2e"/>
  <circle cx="114" cy="51" r="6" fill="#28c840"/>
  <text x="530" y="56" font-family="system-ui, -apple-system, sans-serif" font-size="20" fill="#e6edf3" text-anchor="middle">Homing by MODUS</text>
  <text x="953" y="56" font-family="monospace" font-size="14" fill="#7d8590" text-anchor="middle">remember • recall • attach</text>

  <rect x="74" y="108" width="154" height="34" rx="17" fill="#13213a" stroke="#2f81f7" stroke-width="2"/>
  <text x="151" y="131" font-family="system-ui, -apple-system, sans-serif" font-size="22" fill="#58a6ff" text-anchor="middle">3. Attach</text>
  <text x="256" y="131" font-family="system-ui, -apple-system, sans-serif" font-size="20" fill="#e6edf3">Plain shells and harnesses can ride the same memory.</text>

  <rect x="94" y="182" width="1000" height="98" rx="16" fill="#111b30" stroke="#1f6feb" stroke-width="2"/>
  <text x="122" y="216" font-family="monospace" font-size="21" fill="#e6edf3">$ modus-memory attach --carrier codex</text>
  <text x="122" y="246" font-family="monospace" font-size="21" fill="#e6edf3">  --prompt "What database should we use"</text>
  <text x="122" y="274" font-family="monospace" font-size="21" fill="#e6edf3">  for payments?"</text>

  <rect x="94" y="308" width="1000" height="220" rx="16" fill="#0f172a" stroke="#7ee787" stroke-width="2"/>
  <text x="122" y="344" font-family="monospace" font-size="22" fill="#8b949e">[attach] recalled 3 memory artifacts</text>
  <text x="122" y="378" font-family="monospace" font-size="22" fill="#8b949e">[attach] carrier: codex</text>
  <text x="122" y="412" font-family="monospace" font-size="22" fill="#8b949e">[attach] wrote recall receipt and trace</text>
  <text x="122" y="462" font-family="monospace" font-size="23" fill="#7ee787">Codex:</text>
  <text x="122" y="497" font-family="monospace" font-size="21" fill="#e6edf3">Use Postgres. Homing attached the earlier billing</text>
  <text x="122" y="530" font-family="monospace" font-size="21" fill="#e6edf3">decision and its JSONB rationale before answering.</text>

  <rect x="74" y="578" width="1050" height="40" rx="12" fill="#111827" stroke="#30363d" stroke-width="2"/>
  <text x="599" y="603" font-family="monospace" font-size="16" fill="#8b949e" text-anchor="middle">sovereign memory kernel • plain markdown • one binary</text>
</svg>
EOF
}

write_scene_04() {
  cat >"${TMP_DIR}/scene-04.svg" <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="675" viewBox="0 0 1200 675">
  <rect width="1200" height="675" fill="#0b1020"/>
  <rect x="36" y="28" width="1128" height="619" rx="18" fill="#121a2b" stroke="#263247" stroke-width="2"/>
  <rect x="36" y="28" width="1128" height="46" rx="18" fill="#161b22"/>
  <rect x="36" y="56" width="1128" height="18" fill="#161b22"/>
  <circle cx="70" cy="51" r="6" fill="#ff5f57"/>
  <circle cx="92" cy="51" r="6" fill="#febb2e"/>
  <circle cx="114" cy="51" r="6" fill="#28c840"/>
  <text x="530" y="56" font-family="system-ui, -apple-system, sans-serif" font-size="20" fill="#e6edf3" text-anchor="middle">Homing by MODUS</text>
  <text x="953" y="56" font-family="monospace" font-size="14" fill="#7d8590" text-anchor="middle">remember • recall • attach</text>

  <rect x="94" y="166" width="1000" height="310" rx="18" fill="#101828" stroke="#58a6ff" stroke-width="2"/>
  <text x="180" y="238" font-family="system-ui, -apple-system, sans-serif" font-size="48" fill="#e6edf3">Same memory. Any agent.</text>
  <text x="182" y="308" font-family="monospace" font-size="30" fill="#8b949e">Free for everyone.</text>
  <text x="182" y="352" font-family="monospace" font-size="30" fill="#8b949e">Plain markdown on disk.</text>
  <text x="182" y="396" font-family="monospace" font-size="30" fill="#8b949e">Route-aware recall, not cloud lock-in.</text>

  <rect x="182" y="430" width="160" height="42" rx="12" fill="#13213a"/>
  <rect x="362" y="430" width="190" height="42" rx="12" fill="#13213a"/>
  <rect x="572" y="430" width="196" height="42" rx="12" fill="#13213a"/>
  <rect x="788" y="430" width="206" height="42" rx="12" fill="#13213a"/>
  <text x="262" y="458" font-family="monospace" font-size="21" fill="#7ee787" text-anchor="middle">free for everyone</text>
  <text x="457" y="458" font-family="monospace" font-size="21" fill="#58a6ff" text-anchor="middle">27 tools</text>
  <text x="670" y="458" font-family="monospace" font-size="21" fill="#bc8cff" text-anchor="middle">shell attach</text>
  <text x="891" y="458" font-family="monospace" font-size="21" fill="#e6edf3" text-anchor="middle">plain markdown</text>

  <rect x="74" y="578" width="1050" height="40" rx="12" fill="#111827" stroke="#30363d" stroke-width="2"/>
  <text x="599" y="603" font-family="monospace" font-size="16" fill="#8b949e" text-anchor="middle">sovereign memory kernel • plain markdown • one binary</text>
</svg>
EOF
}

render_scene() {
  local scene_name="$1"
  qlmanage -t -s 1200 -o "${RENDER_DIR}" "${TMP_DIR}/${scene_name}.svg" >/dev/null 2>&1
  ffmpeg -loglevel error -y \
    -i "${RENDER_DIR}/${scene_name}.svg.png" \
    -vf "crop=iw:iw*9/16:0:0" \
    "${RENDER_DIR}/${scene_name}.png"
  rm -f "${RENDER_DIR}/${scene_name}.svg.png"
}

write_scene_01
write_scene_02
write_scene_03
write_scene_04

render_scene scene-01
render_scene scene-02
render_scene scene-03
render_scene scene-04

cat >"${TMP_DIR}/frames.txt" <<EOF
file '${RENDER_DIR}/scene-01.png'
duration 4
file '${RENDER_DIR}/scene-02.png'
duration 4
file '${RENDER_DIR}/scene-03.png'
duration 4
file '${RENDER_DIR}/scene-04.png'
duration 4
file '${RENDER_DIR}/scene-04.png'
EOF

ffmpeg -loglevel error -y \
  -f concat -safe 0 -i "${TMP_DIR}/frames.txt" \
  -vf "fps=6,scale=900:-1:flags=lanczos,split[s0][s1];[s0]palettegen=reserve_transparent=0[p];[s1][p]paletteuse=dither=bayer:bayer_scale=3" \
  -loop 0 \
  "${ROOT_DIR}/assets/demo.gif"

cp "${ROOT_DIR}/assets/demo.gif" "${ROOT_DIR}/cmd/modus-memory/assets/demo.gif"

echo "Rendered demo GIF:"
echo "  ${ROOT_DIR}/assets/demo.gif"
echo "  ${ROOT_DIR}/cmd/modus-memory/assets/demo.gif"
