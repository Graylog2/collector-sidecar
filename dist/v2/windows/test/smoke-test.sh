#!/usr/bin/env bash
# Copyright (C)  2026 Graylog, Inc.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the Server Side Public License, version 1,
# as published by MongoDB, Inc.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# Server Side Public License for more details.
#
# You should have received a copy of the Server Side Public License
# along with this program. If not, see
# <http://www.mongodb.com/licensing/server-side-public-license>.
#
# SPDX-License-Identifier: SSPL-1.0

# Smoke-test the v2 Graylog Collector MSI against a Windows VM reachable over SSH.
#
# Usage:
#   VM=user@winvm MSI=dist/pkg/graylog-collector-2.0.0.msi bash smoke-test.sh <scenario>
#
# Scenarios:
#   1  Fresh silent install with SERVERURL and TOKEN
#   2  Fresh silent install with no properties
#   4  Upgrade with no properties (requires scenario 1 ran first)
#   5  Upgrade with new TOKEN (requires scenario 1 ran first)
#   6  Uninstall (default)
#   7  Uninstall with REMOVE_DATA=1
#
# Scenario 3 (interactive) is manual via RDP/console; not covered by this script.

set -euo pipefail

: "${VM:?VM must be set, e.g., VM=admin@winvm}"
: "${MSI:?MSI must be set, e.g., MSI=dist/pkg/graylog-collector-2.0.0.msi}"

SCENARIO=${1:-}
if [[ -z "$SCENARIO" ]]; then
  echo "Usage: $0 <scenario>" >&2
  exit 2
fi

REMOTE_MSI='C:/Users/Public/graylog-collector.msi'
REMOTE_LOG='C:/Users/Public/install.log'

echo "Copying MSI to $VM..."
scp "$MSI" "$VM:$REMOTE_MSI"

run_ssh() {
  ssh "$VM" "powershell -NoProfile -Command \"$1\""
}

inspect_state() {
  echo ""
  echo "=== Service state ==="
  run_ssh 'Get-Service graylog-collector -ErrorAction SilentlyContinue | Format-List Name,Status,StartType' || true
  echo ""
  echo "=== Service Environment registry ==="
  run_ssh 'Get-ItemProperty -Path HKLM:\SYSTEM\CurrentControlSet\Services\graylog-collector -Name Environment -ErrorAction SilentlyContinue | Format-List Environment' || true
  echo ""
  echo "=== Install state mirror ==="
  run_ssh 'Get-ItemProperty -Path HKLM:\SOFTWARE\Graylog\Collector -ErrorAction SilentlyContinue | Format-List SERVERURL,TOKEN' || true
  echo ""
  echo "=== ProgramData presence ==="
  run_ssh 'Test-Path C:\ProgramData\Graylog\Collector'
  echo ""
  echo "=== Recent application event log ==="
  run_ssh 'Get-WinEvent -LogName Application -MaxEvents 10 -ErrorAction SilentlyContinue | Where-Object { $_.Message -like "*graylog*" -or $_.Message -like "*enrollment*" } | Format-List TimeCreated,LevelDisplayName,Message' || true
}

case "$SCENARIO" in
  1)
    echo "Scenario 1: Fresh silent install with SERVERURL and TOKEN"
    run_ssh "Start-Process msiexec.exe -ArgumentList '/i','$REMOTE_MSI','/qn','/l*v','$REMOTE_LOG','SERVERURL=https://graylog.example.com','TOKEN=test-token-abcdef' -Wait"
    inspect_state
    ;;
  2)
    echo "Scenario 2: Fresh silent install with no properties"
    run_ssh "Start-Process msiexec.exe -ArgumentList '/i','$REMOTE_MSI','/qn','/l*v','$REMOTE_LOG' -Wait"
    inspect_state
    ;;
  4)
    echo "Scenario 4: Upgrade with no properties (pass no SERVERURL/TOKEN)"
    run_ssh "Start-Process msiexec.exe -ArgumentList '/i','$REMOTE_MSI','/qn','/l*v','$REMOTE_LOG' -Wait"
    inspect_state
    ;;
  5)
    echo "Scenario 5: Upgrade with new TOKEN"
    run_ssh "Start-Process msiexec.exe -ArgumentList '/i','$REMOTE_MSI','/qn','/l*v','$REMOTE_LOG','TOKEN=rotated-token-12345' -Wait"
    inspect_state
    ;;
  6)
    echo "Scenario 6: Uninstall (default)"
    run_ssh "Start-Process msiexec.exe -ArgumentList '/x','$REMOTE_MSI','/qn','/l*v','$REMOTE_LOG' -Wait"
    inspect_state
    ;;
  7)
    echo "Scenario 7: Uninstall with REMOVE_DATA=1"
    run_ssh "Start-Process msiexec.exe -ArgumentList '/x','$REMOTE_MSI','/qn','/l*v','$REMOTE_LOG','REMOVE_DATA=1' -Wait"
    inspect_state
    ;;
  *)
    echo "Unknown scenario: $SCENARIO" >&2
    exit 2
    ;;
esac
