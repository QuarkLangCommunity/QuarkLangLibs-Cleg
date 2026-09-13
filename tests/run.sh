#!/usr/bin/env bash
# cleg Style v2 单测入口
# 用法：QUARK=/tmp/quark ./tests/run.sh [过滤词 ...]
# 依赖：cleg.qk（仓库根）、style.qk（STYLE_LIB 或 ../QuarkLangLibs-Style/style.qk）、
#       json.qk（JSON_LIB 或 ../QuarkLangLibs-Json/json.qk）、bin/libclegrt.so
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
QUARK="${QUARK:-quark}"
if [ ! -x "$QUARK" ] && ! command -v "$QUARK" >/dev/null 2>&1; then
  echo "quark 解释器不可用：QUARK=$QUARK" >&2
  exit 2
fi
BUILD="$(mktemp -d /tmp/clegv2-tests.XXXXXX)"
trap 'rm -rf "$BUILD"' EXIT
cp "$ROOT/cleg.qk" "$BUILD/cleg.qk"
JSON_SRC=""
for c in "${JSON_LIB:-}" "$ROOT/json.qk" "$ROOT/../QuarkLangLibs-Json/json.qk" "/home/jack/Projects/QuarkLangLibs-Json/json.qk"; do
  if [ -n "$c" ] && [ -f "$c" ]; then JSON_SRC="$c"; break; fi
done
if [ -z "$JSON_SRC" ]; then
  echo "找不到 json.qk（可用 JSON_LIB=/path/json.qk 指定）" >&2
  exit 2
fi
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
if [ -z "$SO" ]; then
  echo "找不到 libclegrt.so（先构建 native/）" >&2
  exit 2
fi
cp "$SO" "$BUILD/libclegrt.so"

fail=0
libout="$(cd "$BUILD" && "$QUARK" cleg.qk 2>&1)"
if printf '%s' "$libout" | grep -q "cannot run a library"; then
  echo "PASS cleg.qk (parse+typecheck；library 不可运行 = 通过)"
else
  fail=$((fail + 1))
  echo "FAIL cleg.qk"
  printf '%s\n' "$libout" | head -4
fi

total=0
for t in "$ROOT"/tests/*.qk; do
  name="$(basename "$t" .qk)"
  if [ "$#" -gt 0 ]; then
    match=0
    for f in "$@"; do case "$name" in *"$f"*) match=1 ;; esac; done
    [ "$match" = 1 ] || continue
  fi
  total=$((total + 1))
  cp "$t" "$BUILD/$name.qk"
  out="$(cd "$BUILD" && "$QUARK" "$name.qk" 2>&1)"
  rc=$?
  ok="$(printf '%s\n' "$out" | grep -c '^OK')"
  bad="$(printf '%s\n' "$out" | grep -c '^FAIL')"
  if [ "$rc" -ne 0 ] || [ "$bad" -ne 0 ]; then
    fail=$((fail + 1))
    echo "FAIL $name (rc=$rc ok=$ok bad=$bad)"
    printf '%s\n' "$out" | grep -E '^FAIL|error|Error' | head -4
  else
    echo "PASS $name ($ok assertions)"
  fi
done
echo "tests: $total files, failures=$fail"
[ "$fail" -eq 0 ]
