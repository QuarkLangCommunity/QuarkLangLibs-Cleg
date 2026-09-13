#!/usr/bin/env bash
# cleg Style v2 示例入口：产出 PNG，并校验动画 5 帧 md5 互不相同
# 用法：QUARK=/tmp/quark ./examples/run.sh
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
QUARK="${QUARK:-quark}"
BUILD="$(mktemp -d /tmp/clegv2-examples.XXXXXX)"
trap 'rm -rf "$BUILD"' EXIT
cp "$ROOT/cleg.qk" "$BUILD/cleg.qk"
JSON_SRC=""
for c in "${JSON_LIB:-}" "$ROOT/json.qk" "$ROOT/../QuarkLangLibs-Json/json.qk" "/home/jack/Projects/QuarkLangLibs-Json/json.qk"; do
  if [ -n "$c" ] && [ -f "$c" ]; then JSON_SRC="$c"; break; fi
done
[ -n "$JSON_SRC" ] || { echo "找不到 json.qk" >&2; exit 2; }
cp "$JSON_SRC" "$BUILD/json.qk"
STYLE_SRC=""
for c in "${STYLE_LIB:-}" "$ROOT/style.qk" "$ROOT/../QuarkLangLibs-Style/style.qk" "/home/jack/Projects/QuarkLangLibs-Style/style.qk"; do
  if [ -n "$c" ] && [ -f "$c" ]; then STYLE_SRC="$c"; break; fi
done
if [ -z "$STYLE_SRC" ]; then
  echo "找不到 style.qk（通用样式库；可用 STYLE_LIB=/path/style.qk 指定）" >&2
  exit 2
fi
cp "$STYLE_SRC" "$BUILD/style.qk"
SO=""
for s in "$ROOT/bin/libclegrt.so" "$ROOT/native/clegnative"; do
  if [ -f "$s" ]; then SO="$s"; break; fi
done
[ -n "$SO" ] || { echo "找不到 libclegrt.so" >&2; exit 2; }
cp "$SO" "$BUILD/libclegrt.so"

fail=0
for e in style-showcase animation ocean-theme; do
  cp "$ROOT/examples/$e.qk" "$BUILD/$e.qk"
  out="$(cd "$BUILD" && "$QUARK" "$e.qk" 2>&1)"
  rc=$?
  if [ "$rc" -ne 0 ]; then
    fail=$((fail + 1))
    echo "FAIL example $e (rc=$rc)"
    printf '%s\n' "$out" | tail -4
  else
    echo "PASS example $e"
    printf '%s\n' "$out" | tail -6
  fi
done

# 海洋主题帧：6 帧必须全部产出（560ms 后动画已稳定，允许后两帧相同）
mapfile -t OCEAN < <(cd "$BUILD" && ls ocean-*ms.png 2>/dev/null)
if [ "${#OCEAN[@]}" -eq 6 ]; then
  echo "PASS ocean-theme frames: 6/6 produced"
else
  fail=$((fail + 1))
  echo "FAIL ocean-theme frames: ${#OCEAN[@]}/6 files"
fi

# 动画帧差异校验：5 帧 md5 必须互不相同
mapfile -t SUMS < <(cd "$BUILD" && md5sum anim-0.png anim-250.png anim-500.png anim-750.png anim-1000.png 2>/dev/null)
FRAMES=${#SUMS[@]}
UNIQ=$(printf '%s\n' "${SUMS[@]}" | awk '{print $1}' | sort -u | wc -l)
if [ "$FRAMES" -eq 5 ] && [ "$UNIQ" -eq 5 ]; then
  echo "PASS animation frames: 5/5 distinct md5"
else
  fail=$((fail + 1))
  echo "FAIL animation frames: ${FRAMES}/5 files, ${UNIQ} distinct md5"
fi
printf '%s\n' "${SUMS[@]}"

OUT="$ROOT/examples/out"
mkdir -p "$OUT"
cp "$BUILD"/*.png "$OUT/" 2>/dev/null
FRAMES="$ROOT/examples/frames"
mkdir -p "$FRAMES"
cp "$BUILD"/ocean-*ms.png "$FRAMES/" 2>/dev/null
echo "PNG -> $OUT（海洋帧另存 $FRAMES）"
ls -la "$OUT" | tail -8
echo "examples: failures=$fail"
[ "$fail" -eq 0 ]
