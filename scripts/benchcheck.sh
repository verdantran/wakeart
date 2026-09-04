#!/bin/sh
# benchcheck.sh <baseline> <current> <max-percent-regression>
# Compares mean ns/op per benchmark. Exits non-zero on a regression beyond the
# threshold, so CI catches a slow render loop before a user does.
set -eu
baseline=$1; current=$2; limit=${3:-20}

mean() {
  awk -v name="$2" '$1 ~ "^"name"-" { sum += $3; n++ } END { if (n) printf "%.0f", sum/n }' "$1"
}

names=$(awk '/^Benchmark/ { sub(/-[0-9]+$/, "", $1); print $1 }' "$baseline" | sort -u)
status=0
for b in $names; do
  old=$(mean "$baseline" "$b"); new=$(mean "$current" "$b")
  [ -z "$old" ] || [ -z "$new" ] && continue
  pct=$(awk -v o="$old" -v n="$new" 'BEGIN { printf "%.1f", (n-o)*100/o }')
  verdict=ok
  case $(awk -v p="$pct" -v l="$limit" 'BEGIN { print (p > l) }') in
    1) verdict=REGRESSED; status=1 ;;
  esac
  printf '%-28s %10s -> %-10s %+7s%%  %s\n' "$b" "$old" "$new" "$pct" "$verdict"
done
exit $status
