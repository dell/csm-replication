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
ALPHAVERSION="v1alpha1"

echo "ReplicationGroup CRD version migration started."

crdVersion=$(kubectl get customresourcedefinitions dellcsireplicationgroups.replication.storage.dell.com -o=jsonpath='{.status.storedVersions}' 2>&1)
echo -e "ReplicationGroup CRD version found: $crdVersion"
found=false
if [[ $crdVersion == *$ALPHAVERSION* ]]
then
  found=true
fi

if $found
then
  echo "Alpha version found. Starting migration..."

  echo "Step-1: Updating replicationgroup CRD with both v1alpha1 and v1 versions"
  kubectl replace -f "${SCRIPTDIR}"/replicationcrds.v1alpha1-v1.yaml

  echo "Step-2: Migrating replicationgroup CRs to v1 version"
  rgs=$(kubectl get dellcsireplicationgroup -o=jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>&1)
  echo -e "ReplicationGroup IDs:\n$rgs"
  for rg in $rgs
  do
    kubectl get dellcsireplicationgroup $rg -o json | kubectl replace -f -
  done

  echo "Step-3: Updating replicationgroup CRD to remove v1alpha1 from stored versions"
  kubectl patch customresourcedefinitions dellcsireplicationgroups.replication.storage.dell.com --subresource='status' --type='merge' -p '{"status":{"storedVersions":["v1"]}}'

  echo "Step-4: Updating replicationgroup CRD to remove v1alpha1 version"
  kubectl replace -f "${SCRIPTDIR}"/replicationcrds.v1.yaml

  echo "Step-5: Creating new migrationgroup CRD"
  kubectl apply -f "${SCRIPTDIR}"/replicationcrds.migration-v1.yaml
else
  echo "No alpha version found. Skipping migration."
fi

echo "ReplicationGroup CRD version migration completed."
