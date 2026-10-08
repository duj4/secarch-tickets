#!/usr/bin/env bash

QA="https://itsm-qa.lb.ms.com.cn"
UAT="https://itsm-pp.ai.ms.com.cn"

# ID | Test Name | Request Path
CASES=(
  # Normal requests
  "N01|Normal request|/__cve_test__/normal"
  "N02|Harmless dots|/__cve_test__/v1..2/readme"

  # Existing shortlinks
  "S01|Support|/support"
  "S02|Portals|/portals"
  "S03|ITSM lowercase|/itsm"
  "S04|ITSM uppercase|/ITSM"
  "S05|CHG|/CHG"
  "S06|Assets|/assets"
  "S07|CPC|/cpc"

  # Security mitigation
  "B01|Colon traversal|/__cve_test__/::..::probe"
  "B02|Double encoded|/__cve_test__/%252e%252e%252fprobe"
  "B03|Single encoded|/__cve_test__/%2e%2e%2fprobe"

  # Security rules vs shortlinks
  "O01|Support traversal|/support/::..::probe"
  "O02|Portals traversal|/portals/::..::probe"
  "O03|ITSM encoded|/itsm/%252e%252e%252fprobe"
  "O04|ITSM uppercase|/ITSM/::..::probe"
  "O05|CHG traversal|/CHG/::..::probe"
  "O06|Assets traversal|/assets/::..::probe"
  "O07|CPC traversal|/cpc/::..::probe"
)

request() {
  local base="$1"
  local path="$2"

  curl --path-as-is --globoff \
    --connect-timeout 5 \
    --max-time 15 \
    -sS -o /dev/null \
    -w '%{http_code}|%{redirect_url}' \
    "${base}${path}"
}

printf "%-4s %-20s %-48s %-8s %-8s\n" \
  "ID" "TEST" "PATH" "QA" "UAT"

printf '%s\n' "------------------------------------------------------------------------------------------------"

for item in "${CASES[@]}"; do

  IFS='|' read -r id name path <<< "$item"

  qa_result=$(request "$QA" "$path") || qa_result="ERR|"
  uat_result=$(request "$UAT" "$path") || uat_result="ERR|"

  qa_code="${qa_result%%|*}"
  uat_code="${uat_result%%|*}"

  qa_redirect="${qa_result#*|}"
  uat_redirect="${uat_result#*|}"

  printf "%-4s %-20s %-48s %-8s %-8s\n" \
    "$id" "$name" "$path" "$qa_code" "$uat_code"

  # Display redirect destinations for shortlink regression
  if [[ "$id" == S* ]]; then
    printf "     QA  Location: %s\n" "$qa_redirect"
    printf "     UAT Location: %s\n" "$uat_redirect"
  fi

done
