#!/bin/bash

SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
ROOTDIR="$(dirname "$SCRIPTDIR")"
DEPLOYDIR="$ROOTDIR/deploy"

PROG="${0}"

#
# usage will print command execution help and then exit
function print_usage() {
  echo
  echo "Help for $PROG"
  echo
  echo "Usage: $PROG options..."
  echo "Options:"
  echo "  Optional"
  echo "  --image[=]<image-name>             Name of the controller image with tag which will be used for installation"
  echo "  -h                                 Help"
  echo

  exit 0
}

function update_image {
  IMAGE_NAME=$1
  #TODO: Add regex to parse image name to verify if it is valid
  echo "sed -i s/controller:latest/${IMAGE_NAME}/g ${DEPLOYDIR}/controller.yaml"
  sed -i "s/controller:latest/${IMAGE_NAME}/g" "${DEPLOYDIR}"/controller.yaml
}

while getopts 'hi:' flag; do
  case "${flag}" in
    i) update_image $OPTARG;;
    h) print_usage
       exit 0;;
    *) print_usage
       exit 1 ;;
  esac
done


function create_namespace() {
  kubectl get namespace | grep "dell-replication-controller" --quiet
  if [ $? -eq 0 ]; then
    echo "Namespace: dell-replication-controller already exists"
  else
    echo 
    echo "*************************************************"
    echo "Creating namespace dell-replication-controller"
    echo "*************************************************"
    echo "kubectl create namespace dell-replication-controller"
    kubectl create namespace dell-replication-controller
    if [ $? -ne 0 ]; then
      echo "Failed to create namespace: dell-replication-controller. Exiting with failure"
      exit 1
    else
      echo "Create the required secrets for connecting to remote Kubernetes clusters in a separate terminal"
      read -p "Once you have created secrets, Press enter to continue"
    fi
  fi
}

function create_or_update_configmap() {
  echo
  echo "*************************************************"
  echo "Create/Update ConfigMap"
  echo "*************************************************"
  echo "kubectl create configmap dell-replication-controller-config --namespace dell-replication-controller --from-file "$DEPLOYDIR/config.yaml" -o yaml --dry-run | kubectl apply -f -"
  kubectl create configmap dell-replication-controller-config --namespace dell-replication-controller --from-file "$DEPLOYDIR/config.yaml" -o yaml --dry-run | kubectl apply -f -
  if [ $? -ne 0 ]; then
    echo "Failed to create/update ConfigMap for dell-replication-controller. Exiting with failure"
    exit 1
  fi
}

function install_or_update_crds() {
  echo
  echo "*************************************************"
  echo "Install/Update Dell CSI Replication CRDs"
  echo "*************************************************"
  echo "kubectl apply -f ${DEPLOYDIR}/replicationcrds.all.yaml"
  kubectl apply -f ${DEPLOYDIR}/replicationcrds.all.yaml
  if [ $? -ne 0 ]; then
    echo "Failed to install/update Dell CSI Replication CRDs. Exiting with failure"
    exit 1
  fi
}

function create_or_update_deployment() {
  echo
  echo "*************************************************"
  echo "Install Dell Replication Controller"
  echo "*************************************************"
  echo "kubectl apply -f ${DEPLOYDIR}/controller.yaml"
  kubectl apply -f ${DEPLOYDIR}/controller.yaml
  if [ $? -ne 0 ]; then
    echo "Failed to install dell-replication-controller. Exiting with failure"
    exit 1
  fi
  echo
  echo "Successfully deployed Dell Replication Controller"
}

create_namespace
install_or_update_crds
create_or_update_deployment
create_or_update_configmap
