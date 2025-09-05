package test

// Expected output when running `repctl create sc --from-config ...` using a powerscale config template.
// Used to validate repctl is generating the expected storage classes.
// If the config file is updated with more configuration options, the new output should be reviewed and
// upon approval, be used to replace these values.
const powerscaleStorageClass = `
# yamllint disable-file
# This file is not valid YAML because it is a Helm template
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
