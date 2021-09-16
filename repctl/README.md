# Repctl

`repctl` is a command-line client for configuring replication 
and managing replicated resources between multiple Kubernetes clusters.


## Usage

### Managing Clusters

To begin managing replication with `repctl` you need to add your Kubernetes
clusters, you can do that using `cluster add` command

```shell
./repctl cluster add -f <config-file> -n <name>
```

You can view clusters that are currently being managed by `repctl`
by running `cluster list` command

```shell
./repctl cluster list
```

Also, you can inject information about all of your current clusters as
config maps into the same clusters, so it can be used by `dell-csi-replicator`

```shell 
./repctl cluster inject
```

### Querying Resources

After adding clusters you want to manage with `repctl` you can query
resources from multiple clusters at once using `list` command. 

For example, this command will list all storage classes in all clusters
that currently are being managed by `repctl`

```shell
./repctl list storageclasses --all
```

If you want to query some particular clusters you can do that by specifying
`clusters` flag

```shell
./repctl list pv --clusters cluster-1,cluster-3
```

All other different flags for querying resources you can check using
included into the tool help flag `-h`.

### Creating Resources

#### Generic
Generic `create` command allows you to apply provided config file into 
multiple clusters at once

```shell
/repctl create -f <path-to-file>
```

#### PersistentVolumeClaims
You can use `repctl` to create PVCs from Replication Group's PVs 
on the target cluster

```shell
./repctl create pvc --rg <rg-name> -t <target-namespace> --dry-run=false
```

> By default, 'create pvc' will do a 'dry-run' while creating PVCs.
If you don't encounter any issues in the dry-run, then you can
re-run the command by turning off the dry-run flag to false.

#### Storage Classes
`repctl` can create special `replication enabled` storage classes from
provided config, you can find example configs in `examples` folder

```shell
./repctl create sc --from-config <config-file>`
```

### Single Cluster Replication
`repctl` supports working with replication within a single Kubernetes cluster. 

Just add cluster you want to use with `cluster add` command, and you can list, filter, and create resources. 

Volumes and ReplicationGroups created as "target" resources would be prefixed with `replicated-` 
so you can easily differentiate them. 

You can also differentiate between single cluster replication configured StorageClasses and ReplicationGroups and multi-cluster ones 
by checking `remoteClusterID` field, for a single cluster the field would be set to `self`.

To create replication enabled storage classes for single cluster replication using `create sc` command
be sure to set both `sourceClusterID` and `targetClusterID` to the same `clusterID` and continue as usual with executing the command.
Name of StorageClass resource that created as "target" will be appended with `-tgt`. 

