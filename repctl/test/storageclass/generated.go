/*
 Copyright © 2025 Dell Inc. or its subsidiaries. All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
      http://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package test

// Expected output when running `repctl create sc --from-config ...` using a powerscale config template.
// Used to validate repctl is generating the expected storage classes.
// If the config file is updated with more configuration options, the new output should be reviewed and
// upon approval, be used to replace these values.
const powerscaleStorageClass = `
# yamllint disable-file
# This file is not valid YAML because it is a Helm template

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

apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: isilon-replication
provisioner: csi-isilon.dellemc.com
reclaimPolicy: Delete
volumeBindingMode: Immediate
parameters:
  replication.storage.dell.com/isReplicationEnabled: "true"
  replication.storage.dell.com/remoteStorageClassName: isilon-replication
  replication.storage.dell.com/remoteClusterID: target
  replication.storage.dell.com/rpo: Five_Minutes
  replication.storage.dell.com/ignoreNamespaces: "false"
  replication.storage.dell.com/volumeGroupPrefix: csi
  replication.storage.dell.com/remoteSystem: cluster-2
  replication.storage.dell.com/remoteRGRetentionPolicy: Retain
  replication.storage.dell.com/remotePVRetentionPolicy: Retain
  replication.storage.dell.com/remoteAccessZone: System
  replication.storage.dell.com/remoteAzServiceIP: 192.168.1.2
  replication.storage.dell.com/remoteAzNetwork: 192.168.3.0/24
  replication.storage.dell.com/remoteRootClientEnabled: "false"
  AccessZone: System
  AzServiceIP: 192.168.1.1
  AzNetwork: 192.168.2.0/24
  IsiPath: /ifs/data/csi
  RootClientEnabled: "false"
  ClusterName: cluster-1
# yamllint disable-file
# This file is not valid YAML because it is a Helm template

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

apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: isilon-replication
provisioner: csi-isilon.dellemc.com
reclaimPolicy: Delete
volumeBindingMode: Immediate
parameters:
  replication.storage.dell.com/isReplicationEnabled: "true"
  replication.storage.dell.com/remoteStorageClassName: isilon-replication
  replication.storage.dell.com/remoteClusterID: source
  replication.storage.dell.com/rpo: Five_Minutes
  replication.storage.dell.com/ignoreNamespaces: "false"
  replication.storage.dell.com/volumeGroupPrefix: csi
  replication.storage.dell.com/remoteSystem: cluster-1
  replication.storage.dell.com/remoteRGRetentionPolicy: Retain
  replication.storage.dell.com/remotePVRetentionPolicy: Retain
  replication.storage.dell.com/remoteAccessZone: System
  replication.storage.dell.com/remoteAzServiceIP: 192.168.1.1
  replication.storage.dell.com/remoteAzNetwork: 192.168.2.0/24
  replication.storage.dell.com/remoteRootClientEnabled: "false"
  AccessZone: System
  AzServiceIP: 192.168.1.2
  AzNetwork: 192.168.3.0/24
  IsiPath: /ifs/data/csi
  RootClientEnabled: "false"
  ClusterName: cluster-2
`

const powerstoreStorageClass = `
# yamllint disable-file
# This file is not valid YAML because it is a Helm template

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

apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: powerstore-replication
provisioner: csi-powerstore.dellemc.com
reclaimPolicy: Retain
volumeBindingMode: Immediate
parameters:
  replication.storage.dell.com/isReplicationEnabled: "true"
  replication.storage.dell.com/remoteStorageClassName: powerstore-replication
  replication.storage.dell.com/remoteClusterID: cluster-2
  replication.storage.dell.com/remoteSystem: RT-A0002
  replication.storage.dell.com/rpo: Five_Minutes
  replication.storage.dell.com/mode: ASYNC
  replication.storage.dell.com/ignoreNamespaces: "false"
  replication.storage.dell.com/volumeGroupPrefix: csi
  replication.storage.dell.com/remoteRGRetentionPolicy: Retain
  replication.storage.dell.com/remotePVRetentionPolicy: Retain
  arrayID: PS000000000001
# yamllint disable-file
# This file is not valid YAML because it is a Helm template

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

apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: powerstore-replication
provisioner: csi-powerstore.dellemc.com
reclaimPolicy: Retain
volumeBindingMode: Immediate
parameters:
  replication.storage.dell.com/isReplicationEnabled: "true"
  replication.storage.dell.com/remoteStorageClassName: powerstore-replication
  replication.storage.dell.com/remoteClusterID: cluster-1
  replication.storage.dell.com/remoteSystem: RT-A0001
  replication.storage.dell.com/rpo: Five_Minutes
  replication.storage.dell.com/mode: ASYNC
  replication.storage.dell.com/ignoreNamespaces: "false"
  replication.storage.dell.com/volumeGroupPrefix: csi
  replication.storage.dell.com/remoteRGRetentionPolicy: Retain
  replication.storage.dell.com/remotePVRetentionPolicy: Retain
  arrayID: PS000000000002
`
