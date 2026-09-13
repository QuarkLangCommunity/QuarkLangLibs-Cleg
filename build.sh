#!/bin/sh
# 顶层构建入口：调用 native/build.sh 产出 bin/libclegrt.so。
set -eu
exec "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/native/build.sh" "$@"
