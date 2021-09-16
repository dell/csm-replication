#!/bin/bash
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
