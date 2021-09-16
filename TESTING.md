# Testing
There are many unit tests written in the respective packages in `_test.go` files, Along with the unit tests there are integration tests written separately for both Sidecar and Replication controller. These integration tests cover complete end to end scenario testing starting from automated way of creating the resources PVC, PV and RG and by running the sidecar/replication controllers internally from the tests itself.

The integration test cases can be executed by using the `Makefile` recipe or from any IDE directly. These tests can be ran against any driver without any modification to the code by just updating the driver and the storage class to use in the stubs and in the integration tests config variable.

```
	suite.driver = common.Driver{
		DriverName:   "csi-unity.dellemc.com",
		StorageClass: "replication-unity",
	}
```

To run sidecar integration tests: 

```bash
make run-sidecar-tests
```

To run replication controller tests:

Set the remote cluster config file path into the environment variable and run the tests.

```bash
export REMOTE_KUBE_CONFIG_PATH=C:\config-files\remote-cluster-config.yaml
make run-controller-tests
```

To run unit tests: 
```bash
make gen-coverage
```
