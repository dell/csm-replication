# Testing
There are many unit tests written in the respective packages in `_test.go` files. Along with the unit tests there are integration tests written separately for both Sidecar and Replication controller. These integration tests cover complete end to end scenario testing starting from automated way of creating the resources PVC, PV and RG and by running the sidecar/replication controllers internally from the tests itself.

## Integration Tests
The integration test cases can be executed by using the `Makefile` recipe or from any IDE directly. These tests can be ran against any driver without any modification to the code by just updating the driver and the storage class to use in the stubs and in the integration tests config variable.

```
	suite.driver = common.Driver{
		DriverName:   "csi-unity.dellemc.com",
		StorageClass: "replication-unity",
	}
```
### Sidecar
To run sidecar integration tests: 
- Start the mock gRPC server based on the dev environment operating system.

	For Linux distros:
	```bash
	make start-server-unix
	```

	For Windows:

	```bash
	make start-server-win
	```

- Open a new terminal window and run the tests.

	```bash
	make run-sidecar-tests
	```

### Replication Controller Manager
To run replication controller tests:

Set `REMOTE_KUBE_CONFIG_PATH` to the path to the remote cluster's kubeconfig and run the tests.

```bash
export REMOTE_KUBE_CONFIG_PATH=$HOME/.kube/config-remote
make run-controller-tests
```

## Unit Tests
To run unit tests for CSM Replication: 

```bash
make unit-test
```

### Repctl
To run unit test for repctl:

```bash
cd ./repctl
make test
```