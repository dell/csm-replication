#!/bin/bash
#
# Copyright (c) 2021 Dell Inc., or its subsidiaries. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0

#######################################
# Main entry point
# Globals:
#   debuglog - path to debug log
#   scriptdir - scripts location
# Arguments:
#  None
#######################################
function main() {
  scriptdir="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
  source "./common.sh"

  # export the name of the debug log, so child processes will see it
  export debuglog="${scriptdir}/uninstall-debug.log"

  if [ -f "${debuglog}" ]; then
    rm -f "${debuglog}"
  fi

  run_command helm delete -n "dell-replication-controller" "replication"

  if [ $? -ne 0 ]; then
    decho "Removal of the CSM Replication was unsuccessful"
    read -p "Do you want to try to remove the installation using plain manifests from /deploy directory? [Y/N]: " cont
    local res=$(echo "$cont" | tr '[:upper:]' '[:lower:]')
    if [[ $res  = "y" ]] || [[ $res  = "yes" ]]  ; then
      run_command kubectl delete -f "$scriptdir/../deploy/controller.yaml"
      exit 0
    else
      echo "Exiting."
      exit 1
    fi
  fi

  decho "Removal of the CSI Driver is in progress."

  decho "It may take a few minutes for all pods to terminate."

}

main "$@"
