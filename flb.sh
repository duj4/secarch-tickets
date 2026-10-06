#!/bin/sh
FLB_PID="${FLB_PID:-1}"
FLB_BIN="${FLB_BIN:-/fluent-bit/bin/fluent-bit}"
STORAGE_PATH="${STORAGE_PATH:-/var/log/fluent-bit-storage}"

section(){ echo; echo '##################################################'; echo "### $1"; echo '##################################################'; }
subsection(){ echo; echo "===== $1 ====="; }

section "BASIC"
date
hostname

subsection "FLUENT BIT PROCESS"
ps -ef 2>/dev/null | grep '[f]luent-bit' || true

subsection "FLUENT BIT VERSION"
"$FLB_BIN" --version 2>&1 || true

subsection "BUILD FLAGS / ALLOCATOR"
"$FLB_BIN" -h 2>&1 | grep -iE 'Build Flags|JEMALLOC|VALGRIND' || true

subsection "ALLOCATOR ENVIRONMENT"
tr '\0' '\n' < "/proc/$FLB_PID/environ" 2>/dev/null \
  | grep -iE '^(MALLOC_CONF|MALLOC|JEMALLOC|GLIBC|LD_PRELOAD)=' || true

section "PROCESS STATUS"
grep -E '^(Name|Pid|PPid|Threads|VmPeak|VmSize|VmRSS|VmData|VmStk|VmExe|VmLib|VmSwap|RssAnon|RssFile|RssShmem):' \
  "/proc/$FLB_PID/status" 2>/dev/null || true

section "PROCESS SMAPS ROLLUP"
grep -E '^(Rss|Pss|Pss_Anon|Pss_File|Pss_Shmem|Shared_Clean|Shared_Dirty|Private_Clean|Private_Dirty|Referenced|Anonymous|LazyFree|AnonHugePages|ShmemPmdMapped|FilePmdMapped|Shared_Hugetlb|Private_Hugetlb|Swap|SwapPss|Locked):' \
  "/proc/$FLB_PID/smaps_rollup" 2>/dev/null || true

section "PROCESS MAP SUMMARY"
echo -n 'Total mappings: '
wc -l < "/proc/$FLB_PID/maps" 2>/dev/null || true

echo -n 'Mappings with no file backing: '
awk '
/^[0-9a-fA-F]+-[0-9a-fA-F]+ / {
    if (NF < 6 || $6 ~ /^\[/) count++
}
END { print count + 0 }
' "/proc/$FLB_PID/maps" 2>/dev/null || true

echo -n 'Mappings with Anonymous > 0: '
awk '
function flush(){ if (have && anon > 0) count++ }
/^[0-9a-fA-F]+-[0-9a-fA-F]+ / { flush(); have=1; anon=0; next }
/^Anonymous:/ { anon=$2 }
END { flush(); print count + 0 }
' "/proc/$FLB_PID/smaps" 2>/dev/null || true

section "TOP MAPPINGS BY ANONYMOUS MEMORY"
awk '
function flush(){
    if (have && anon > 0) {
        printf "%12d %12d %16d %16d %12d  %s\n", anon, rss, pdirty, ahp, size, hdr
    }
}
/^[0-9a-fA-F]+-[0-9a-fA-F]+ / {
    flush(); have=1; hdr=$0; size=rss=anon=pdirty=ahp=0; next
}
/^Size:/          { size=$2 }
/^Rss:/           { rss=$2 }
/^Private_Dirty:/ { pdirty=$2 }
/^Anonymous:/     { anon=$2 }
/^AnonHugePages:/ { ahp=$2 }
END { flush() }
' "/proc/$FLB_PID/smaps" 2>/dev/null \
| sort -nr -k1,1 \
| head -30 \
| awk 'BEGIN { printf "%12s %12s %16s %16s %12s  %s\n","Anon_kB","RSS_kB","PrivateDirty_kB","AnonHuge_kB","Size_kB","Mapping" } { print }' || true

section "TOP MAPPINGS BY RSS"
awk '
function flush(){
    if (have && rss > 0) {
        printf "%12d %12d %16d %16d %12d  %s\n", rss, anon, pdirty, ahp, size, hdr
    }
}
/^[0-9a-fA-F]+-[0-9a-fA-F]+ / {
    flush(); have=1; hdr=$0; size=rss=anon=pdirty=ahp=0; next
}
/^Size:/          { size=$2 }
/^Rss:/           { rss=$2 }
/^Private_Dirty:/ { pdirty=$2 }
/^Anonymous:/     { anon=$2 }
/^AnonHugePages:/ { ahp=$2 }
END { flush() }
' "/proc/$FLB_PID/smaps" 2>/dev/null \
| sort -nr -k1,1 \
| head -30 \
| awk 'BEGIN { printf "%12s %12s %16s %16s %12s  %s\n","RSS_kB","Anon_kB","PrivateDirty_kB","AnonHuge_kB","Size_kB","Mapping" } { print }' || true

section "HEAP / STACK MAPPING DETAIL"
awk '
function print_map(){
    if (selected) {
        print hdr
        printf "Size:           %d kB\n", size
        printf "Rss:            %d kB\n", rss
        printf "Pss:            %d kB\n", pss
        printf "Private_Dirty:  %d kB\n", pdirty
        printf "Anonymous:      %d kB\n", anon
        printf "AnonHugePages:  %d kB\n", ahp
        print ""
    }
}
/^[0-9a-fA-F]+-[0-9a-fA-F]+ / {
    print_map()
    hdr=$0
    selected = ($0 ~ /\[heap\]/ || $0 ~ /\[stack\]/)
    size=rss=pss=pdirty=anon=ahp=0
    next
}
/^Size:/          { if (selected) size=$2 }
/^Rss:/           { if (selected) rss=$2 }
/^Pss:/           { if (selected) pss=$2 }
/^Private_Dirty:/ { if (selected) pdirty=$2 }
/^Anonymous:/     { if (selected) anon=$2 }
/^AnonHugePages:/ { if (selected) ahp=$2 }
END { print_map() }
' "/proc/$FLB_PID/smaps" 2>/dev/null || true

section "TRANSPARENT HUGE PAGES"
for f in \
  /sys/kernel/mm/transparent_hugepage/enabled \
  /sys/kernel/mm/transparent_hugepage/defrag \
  /sys/kernel/mm/transparent_hugepage/shmem_enabled
do
    if [ -r "$f" ]; then
        echo "$f:"
        cat "$f"
    fi
done

section "CGROUP"
cat "/proc/$FLB_PID/cgroup" 2>/dev/null || true

CG_REL=$(awk -F: '$1=="0" {print $3}' "/proc/$FLB_PID/cgroup" 2>/dev/null)
CG="/sys/fs/cgroup${CG_REL}"

echo "CG_REL=$CG_REL"
echo "CG=$CG"

subsection "MEMORY CURRENT / MAX"
MEM_CURRENT=$(cat "$CG/memory.current" 2>/dev/null || echo 0)
MEM_MAX=$(cat "$CG/memory.max" 2>/dev/null || echo unknown)
echo "memory.current=$MEM_CURRENT"
echo "memory.max=$MEM_MAX"
case "$MEM_CURRENT" in
  ''|*[!0-9]*) ;;
  *) awk -v x="$MEM_CURRENT" 'BEGIN {printf "memory.current=%.1f MiB\n", x/1048576}' ;;
esac

subsection "MEMORY STAT RAW"
grep -E '^(anon|file|kernel|kernel_stack|pagetables|percpu|sock|vmalloc|shmem|file_mapped|file_dirty|file_writeback|swapcached|anon_thp|file_thp|shmem_thp|inactive_anon|active_anon|inactive_file|active_file|slab_reclaimable|slab_unreclaimable|slab) ' \
  "$CG/memory.stat" 2>/dev/null || true

subsection "MEMORY STAT MiB"
grep -E '^(anon|file|kernel|kernel_stack|pagetables|percpu|sock|vmalloc|shmem|file_mapped|file_dirty|file_writeback|swapcached|anon_thp|file_thp|shmem_thp|inactive_anon|active_anon|inactive_file|active_file|slab_reclaimable|slab_unreclaimable|slab) ' \
  "$CG/memory.stat" 2>/dev/null \
| awk '{printf "%-22s %10.1f MiB\n", $1, $2/1048576}' || true

subsection "MEMORY EVENTS"
cat "$CG/memory.events" 2>/dev/null || true

subsection "MEMORY PRESSURE"
cat "$CG/memory.pressure" 2>/dev/null || true

subsection "MEMORY NUMA STAT"
cat "$CG/memory.numa_stat" 2>/dev/null || true

section "FD / THREADS"
FD_COUNT=$(ls "/proc/$FLB_PID/fd" 2>/dev/null | wc -l)
echo "FD count: $FD_COUNT"
grep '^Threads:' "/proc/$FLB_PID/status" 2>/dev/null || true

subsection "FD BREAKDOWN"
for f in /proc/"$FLB_PID"/fd/*; do
    x=$(readlink "$f" 2>/dev/null || true)
    case "$x" in
      "$STORAGE_PATH"/*) echo "flb_storage" ;;
      /var/log/containers/*) echo "container_log" ;;
      socket:*) echo "socket" ;;
      pipe:*) echo "pipe" ;;
      anon_inode:*) echo "anon_inode" ;;
      /dev/*) echo "device" ;;
      "") echo "unreadable" ;;
      *) echo "other" ;;
    esac
done | sort | uniq -c | sort -nr

subsection "TOP FD TARGETS"
for f in /proc/"$FLB_PID"/fd/*; do
    readlink "$f" 2>/dev/null || true
done | sed 's/ (deleted)$//' | sort | uniq -c | sort -nr | head -50

subsection "DELETED FDs"
DELETED_FOUND=0
for f in /proc/"$FLB_PID"/fd/*; do
    x=$(readlink "$f" 2>/dev/null || true)
    case "$x" in
      *"(deleted)"*) echo "$f -> $x"; DELETED_FOUND=1 ;;
    esac
done
[ "$DELETED_FOUND" -eq 0 ] && echo "None"

section "STORAGE"
du -sh "$STORAGE_PATH" 2>/dev/null || true
echo -n 'files: '
find "$STORAGE_PATH" -type f 2>/dev/null | wc -l
echo -n 'dirs: '
find "$STORAGE_PATH" -type d 2>/dev/null | wc -l

subsection "STORAGE FILE SAMPLE"
find "$STORAGE_PATH" -type f 2>/dev/null | head -100 || true

subsection "STORAGE FILE COUNTS BY TOP-LEVEL DIRECTORY"
find "$STORAGE_PATH" -mindepth 2 -type f 2>/dev/null \
| sed "s#^$STORAGE_PATH/##" \
| awk -F/ '{print $1}' \
| sort | uniq -c | sort -nr | head -50 || true

subsection "FILESYSTEM"
df -h "$STORAGE_PATH" 2>/dev/null || true
df -i "$STORAGE_PATH" 2>/dev/null || true
mount 2>/dev/null | grep -E 'fluent-bit-storage|/var/log' || true

section "HOST MEMORY / SLAB"
grep -E '^(MemTotal|MemFree|MemAvailable|Cached|Buffers|Slab|SReclaimable|SUnreclaim|AnonPages|AnonHugePages|HugePages_Total|HugePages_Free):' \
  /proc/meminfo 2>/dev/null || true

subsection "HOST SLAB SELECTED"
grep -E '^(dentry|inode_cache|ext4_inode_cache|xfs_inode|buffer_head|radix_tree_node)' \
  /proc/slabinfo 2>/dev/null || true

section "END"
date

