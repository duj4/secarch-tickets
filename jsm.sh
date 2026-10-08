#!/bin/bash

QA="https://<QA-VIP>"
UAT="https://<UAT-VIP>"

paths=(
  "/__cve_test__/normal"
  "/__cve_test__/::..::probe"
  "/__cve_test__/%2e%2e%2fprobe"
  "/__cve_test__/%252e%252e%252fprobe"
)

for base in "$QA" "$UAT"; do
  echo
  echo "========== $base =========="

  for path in "${paths[@]}"; do
    printf "%-55s " "$path"

    curl --path-as-is \
      --connect-timeout 5 \
      --max-time 15 \
      -sS -o /dev/null \
      -w "HTTP %{http_code}  Redirect: %{redirect_url}\n" \
      "${base}${path}"
  done
done
