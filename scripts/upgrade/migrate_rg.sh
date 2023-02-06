#!/bin/bash
#
######################################################################
# Copyright © 2023 Dell Inc. or its subsidiaries. All Rights Reserved.
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

set -e

SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
RGVERSION="replication.storage.dell.com/v1alpha1"

echo "ReplicationGroup version migration started."

rgVersions=$(kubectl get rg -o=jsonpath='{range .items[*]}{.apiVersion}{"\n"}{end}' 2>&1)
echo -e "ReplicationGroup API versions found:\n$rgVersions"
found=false
for rgVersion in $rgVersions
do
  if [ $rgVersion == $RGVERSION ]
  then
    found=true
    break
  fi
done


if $found
then
  echo "Alpha versions found. Starting migration..."

  echo "Step-1: Updating CRD with both v1alpha1 and v1 versions"
  kubectl replace -f "${SCRIPTDIR}"/replicationcrds.v1alpha1-v1.yaml

  echo "Step-2: Migrating ReplicationGroups to v1 version"
  rgs=$(kubectl get rg -o=jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>&1)
  echo -e "ReplicationGroup IDs:\n$rgs"
  for rg in $rgs
  do
    kubectl get rg $rg -o json | kubectl replace -f -
  done

  echo "Step-3: Updating CRD to remove v1alpha1 from stored versions"
  kubectl patch customresourcedefinitions dellcsireplicationgroups.replication.storage.dell.com --subresource='status' --type='merge' -p '{"status":{"storedVersions":["v1"]}}'

  echo "Step-4: Updating CRD to remove v1alpha1 version"
  kubectl replace -f "${SCRIPTDIR}"/replicationcrds.v1.yaml
else
  echo "No alpha versions found. Skipping migration."
fi

echo "ReplicationGroup version migration completed."
