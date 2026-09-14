#!/usr/bin/env bash
# Usage: bash restore_go_files.sh [directory]
# Defaults to the directory containing this script.
set -euo pipefail

if (( $# > 1 )); then
    printf '用法：bash %s [目录]\n' "$0" >&2
    exit 1
fi

rename_root=${1:-"$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"}
if [[ ! -d "$rename_root" ]]; then
    printf '目录不存在：%s\n' "$rename_root" >&2
    exit 1
fi
rename_root=$(cd -- "$rename_root" && pwd -P)

# Complete the scan before renaming, including checking find's exit status.
rename_sources=()
exec 3< <(find "$rename_root" -type f -name '*.go.txt' -print0)
rename_find_pid=$!
while IFS= read -r -d '' rename_source <&3; do
    rename_sources+=("$rename_source")
done
exec 3<&-
if ! wait "$rename_find_pid"; then
    printf '扫描目录失败，未重命名任何文件。\n' >&2
    exit 1
fi

# Check every destination first so existing files are never overwritten.
for rename_source in "${rename_sources[@]}"; do
    rename_target=${rename_source%.txt}
    if [[ -e "$rename_target" || -L "$rename_target" ]]; then
        printf '目标已存在，未重命名任何文件：%s\n' "$rename_target" >&2
        exit 1
    fi
done

rename_count=0
for rename_source in "${rename_sources[@]}"; do
    rename_target=${rename_source%.txt}
    mv -n -- "$rename_source" "$rename_target"
    if [[ -e "$rename_source" || -L "$rename_source" ]]; then
        printf '文件未能重命名，已停止：%s\n' "$rename_source" >&2
        exit 1
    fi
    rename_count=$((rename_count + 1))
done

printf '已将 %d 个 .go.txt 文件改为 .go。\n' "$rename_count"
