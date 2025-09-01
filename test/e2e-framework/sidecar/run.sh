#!/bin/bash

# Copyright © 2020-2025 Dell Inc. or its subsidiaries. All Rights Reserved.
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
#

#!/bin/bash
######################################################################
# Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.
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

# This script is invoked by the Makefile target which sets the required env vars
# Make sure that the driver is running and listening at the same socket file
# Edit env.sh file before running these tests

# Set the required env vars
. ./env.sh
command -v podman
if [ $? -eq 0 ]; then
  echo "Using podman for running provisioner container"
  CONTAINER_TOOL="podman"
else
  command -v docker
  if [ $? -eq 0 ]; then
    echo "Using docker for building image"
    CONTAINER_TOOL="docker"
  else
    echo "Failed to find podman or docker  for running provisioner container. Exiting with failure"
    exit 1
  fi
fi

PROVISIONER_NAME=sa-csi-provisioner
# Start the provisioner container
echo $CONTAINER_TOOL run --rm -v $CSI_SOCK_DIR:/var/run/csi -v $KUBECONFIG_PATH:/kubeconfig --privileged -d --name $PROVISIONER_NAME $PROVISIONER_IMAGE --kubeconfig /kubeconfig --csi-address="/var/run/csi/csi.sock" --volume-name-prefix=$VOL_PREFIX --volume-name-uuid-length=$VOL_UUID_LENGTH --timeout=180s --worker-threads=6 --v=5 --default-fstype=ext4 --extra-create-metadata --feature-gates=Topology=true
$CONTAINER_TOOL run --rm -v $CSI_SOCK_DIR:/var/run/csi -v $KUBECONFIG_PATH:/kubeconfig --privileged -d --name $PROVISIONER_NAME $PROVISIONER_IMAGE --kubeconfig /kubeconfig --csi-address="/var/run/csi/csi.sock" --volume-name-prefix=$VOL_PREFIX --volume-name-uuid-length=$VOL_UUID_LENGTH --timeout=180s --worker-threads=6 --v=5 --default-fstype=ext4 --extra-create-metadata --feature-gates=Topology=true

# Start the tests
echo go test -v
go test -v
# Stop the provisioner container
# Since the container was started with --rm flag, it will be automatically deleted once stopped
echo $CONTAINER_TOOL stop $PROVISIONER_NAME
$CONTAINER_TOOL stop $PROVISIONER_NAME
