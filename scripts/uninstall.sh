#!/bin/bash
#
######################################################################
# Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#      http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
######################################################################

#######################################
# Main entry point
# Globals:
#   debuglog - path to debug log
#   scriptdir - scripts location
# Arguments:
#  None
#######################################

SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
NS="dell-replication-controller"
RELEASE="replication"

# export the name of the debug log, so child processes will see it
export DEBUGLOG="${SCRIPTDIR}/uninstall-debug.log"

source "$SCRIPTDIR"/common.sh

if [ -f "${DEBUGLOG}" ]; then
  rm -f "${DEBUGLOG}"
fi

function uninstall_module_manifests() {
  read -p "Do you want to try to remove the installation using plain manifests from /deploy directory? [Y/N]: " cont
  local res=$(echo "$cont" | tr '[:upper:]' '[:lower:]')
  if [[ $res  = "y" ]] || [[ $res  = "yes" ]]  ; then
    run_command kubectl delete -f "$SCRIPTDIR/../deploy/controller.yaml"
    exit 0
  else
    echo "Exiting."
    exit 1
  fi
}

#
# main
#
run_command helm delete -n $NS $RELEASE

if [ $? -ne 0 ]; then
  # helm removal unsuccessful, try using manifests
  decho "Removal of the CSM Replication module was unsuccessful."
  uninstall_module_manifests
fi

decho "Removal of the CSM Replication module is in progress."
decho "It may take a few minutes for all pods to terminate."
