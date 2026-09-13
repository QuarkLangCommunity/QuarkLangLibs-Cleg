#!/bin/sh
# cleg 渲染后端构建脚本：产出 bin/libclegrt.so（+ cgo 生成的 bin/libclegrt.h）。
# 纯 Go、零系统依赖；-buildmode=c-shared 导出 STYLE-SPEC §9 的全部 cleg_* C ABI。
#
# 用法：native/build.sh [输出目录]      （默认 <repo>/bin）
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(dirname -- "$here")
out=${1:-"$root/bin"}
mkdir -p "$out"
cd "$here"

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	echo "gofmt 未通过：" >&2
	echo "$unformatted" >&2
	exit 1
fi

go vet ./...
go build -buildmode=c-shared -o "$out/libclegrt.so" .
echo "built $out/libclegrt.so"
