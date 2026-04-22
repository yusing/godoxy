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
git diff origin/main -- ':(glob)**/*.go' >"$patch_file"
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

# create placeholder files for bun-minified assets (see scripts/minify/index.ts) so go vet won't complain
while IFS= read -r file; do
	bn=$(basename "$file")
	[[ $bn == *".min."* ]] && continue
	case "$file" in internal/go-proxmox/*) continue ;; esac
	ext="${file##*.}"
	base="${file%.*}"
	min_file="${base}.min.${ext}"
	[ -f "$min_file" ] || : >"$min_file"
done < <(find internal/ goutils/ \( -name '*.js' -o -name '*.html' \) 2>/dev/null)

docker_version="$(
	git show origin/compat:go.mod |
		sed -n 's/^[[:space:]]*github.com\/docker\/docker[[:space:]]\+\(v[^[:space:]]\+\).*/\1/p' |
		head -n 1
)"
if [ -n "$docker_version" ]; then
	go mod edit -droprequire=github.com/docker/docker/api || true
	go mod edit -droprequire=github.com/docker/docker/client || true
	go mod edit -require="github.com/docker/docker@${docker_version}"
fi

go mod tidy
go mod -C agent tidy
git add -A
git commit -m "Apply compat patch"
go vet ./...
go vet -C agent ./...
