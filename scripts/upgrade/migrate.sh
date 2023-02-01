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

SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
CRDDIR="${SCRIPTDIR}/../../deploy"
RGVERSION='replication.storage.dell.com/v1alpha1'

echo "RG version migration started."

#rgs=$(kubectl get rg -o=jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.apiVersion}{"\n"}{end}' 2>&1)
rgVersions=$(kubectl get rg -o=jsonpath='{range .items[*]}{.apiVersion}{"\n"}{end}' 2>&1)
echo "RG versions:"
echo "$rgVersions"
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
  echo "Alpha versions found. Starting RG migration..."

  echo "------Step-1------"
  echo "Updating CRD with both v1alpha1 and v1 versions"
  kubectl replace -f "${SCRIPTDIR}"/replicationcrds.all.yaml

  echo "------Step-2------"
  echo "Migrating RGs to v1 version"
  rgs=$(kubectl get rg -o=jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>&1)
  echo "$rgs"
  for rg in $rgs
  do
    kubectl get rg $rg -o json | kubectl replace -f -
  done

  echo "------Step-3------"
  echo "Updating CRD to remove v1alpha1 from stored versions"
  kubectl patch customresourcedefinitions dellcsireplicationgroups.replication.storage.dell.com --subresource='status' --type='merge' -p '{"status":{"storedVersions":["v1"]}}'

  echo "------Step-4------"
  echo "Updating CRD to remove v1alpha1 version"
  kubectl replace -f "${CRDDIR}"/replicationcrds.all.yaml
else
  echo "No alpha versions found. Skipping RG migration."
fi

echo "RG version migration completed."