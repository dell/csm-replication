export CSI_SOCK_DIR="/var/run/csi"
export CSI_SOCK_FILE="csi.sock"
export CSI_ADDRESS=$CSI_SOCK_DIR/$CSI_SOCK_FILE
export KUBECONFIG_PATH="/root/.kube/config"

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
export PROVISIONER_IMAGE="k8s.gcr.io/sig-storage/csi-provisioner:v2.2.1"
export VOL_PREFIX="rep"
export VOL_UUID_LENGTH="10"
