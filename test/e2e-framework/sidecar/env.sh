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

export CSI_SOCK_DIR="/var/run/csi"
export CSI_SOCK_FILE="csi.sock"
export CSI_ADDRESS=$CSI_SOCK_DIR/$CSI_SOCK_FILE
export KUBECONFIG_PATH=$HOME/.kube/config

export PROBE_TIMEOUT="10s"
export TIMEOUT="180s"
export TEST_TIMEOUT="5m"
export RETRY_START_INTERVAL="1s"
export RETRY_MAX_INTERVAL="5m"
export DOMAIN="replication.storage.dell.com"
export CONTEXT_PREFIX="replication"
export WORKERS="2"
export NAMESPACE="test"
export STORAGE_CLASS="replicated-sc"

# Provisioner settings
export PROVISIONER_IMAGE="registry.k8s.io/sig-storage/csi-provisioner:v2.2.1"
export VOL_PREFIX="rep"
export VOL_UUID_LENGTH="10"
