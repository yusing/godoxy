#!/bin/bash
set -euo pipefail

if ! git diff --quiet || ! git diff --cached --quiet; then
	echo "Working tree is not clean. Commit or stash changes before running refresh-compat.sh." >&2
	exit 1
fi

git fetch origin main compat
git checkout -B compat origin/compat
patch_file="$(mktemp)"
trap 'rm -f "$patch_file"' EXIT
git diff origin/main -- . ':(exclude)**/go.mod' ':(exclude)**/go.sum' >"$patch_file"
git checkout -B main origin/main
git branch -D compat
git checkout -b compat
git apply "$patch_file"
mapfile -t changed_go_files < <(git diff --name-only -- '*.go')
fmt_go_files=()
for file in "${changed_go_files[@]}"; do
	[ -f "$file" ] || continue
	sed -i 's/sonic\./json\./g' "$file"
	sed -i 's/"github.com\/bytedance\/sonic"/"encoding\/json"/g' "$file"
	sed -E -i 's/\bsonic[[:space:]]+"encoding\/json"/json "encoding\/json"/g' "$file"
	fmt_go_files+=("$file")
done
if [ "${#fmt_go_files[@]}" -gt 0 ]; then
	gofmt -w "${fmt_go_files[@]}"
fi
git add -A
git commit -m "Apply compat patch"
go vet ./...
go vet -C agent ./...
