#!/bin/bash

git fetch origin main compat
git checkout -B compat origin/compat
patch_file="$(mktemp)"
git diff origin/main -- . ':(exclude)**/go.mod' ':(exclude)**/go.sum' >"$patch_file"
sed -i 's/sonic\./json\./g' "$patch_file"
sed -i 's/"github.com\/bytedance\/sonic"/"encoding\/json"/g' "$patch_file"
git checkout -B main origin/main
git branch -D compat
git checkout -b compat
git apply "$patch_file"
mapfile -t changed_go_files < <(git diff --name-only -- '*.go')
fmt_go_files=()
for file in "${changed_go_files[@]}"; do
	[ -f "$file" ] || continue
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
