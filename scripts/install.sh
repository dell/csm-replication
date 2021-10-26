#!/bin/bash
#
# Copyright (c) 2020 Dell Inc., or its subsidiaries. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0

SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DRIVERDIR="${SCRIPTDIR}/../helm"
PROG="${0}"
MODE="install"
WATCHLIST=""

# export the name of the debug log, so child processes will see it
export DEBUGLOG="${SCRIPTDIR}/install-debug.log"
declare -a VALIDDRIVERS

source "$SCRIPTDIR"/common.sh

if [ -f "${DEBUGLOG}" ]; then
  rm -f "${DEBUGLOG}"
fi

#
# usage will print command execution help and then exit
function usage() {
  decho
  decho "Help for $PROG"
  decho
  decho "Usage: $PROG options..."
  decho "Options:"
  decho "  Required"
  decho "  --namespace[=]<namespace>                Kubernetes namespace for installation"
  decho "  --values[=]<values.yaml>                 Values file, which defines configuration values"

  decho "  Optional"
  decho "  --release[=]<helm release>               Name to register with helm, default value will match the CSM-Replication name"
  decho "  --upgrade                                Perform an upgrade, default is false"
  decho "  -h                                       Help"
  decho

  exit 0
}

# warning, with an option for users to continue
function warning() {
  log separator
  printf "${YELLOW}WARNING:${NC}\n"
  for N in "$@"; do
    decho $N
  done
  decho
  if [ "${ASSUMEYES}" == "true" ]; then
    decho "Continuing as '-Y' argument was supplied"
    return
  fi
  read -n 1 -p "Press 'y' to continue or any other key to exit: " CONT
  decho
  if [ "${CONT}" != "Y" -a "${CONT}" != "y" ]; then
    decho "quitting at user request"
    exit 2
  fi
}


# print header information
function header() {
  log section "Installing CSM Replication ${DRIVER} on ${kMajorVersion}.${kMinorVersion}"
}

#
# check_for_driver will see if the driver is already installed within the namespace provided
function check_for_driver() {
  log section "Checking to see if Module is already installed"
  NUM=$(run_command helm list --namespace "${NS}" | grep "^${RELEASE}\b" | wc -l)
  if [ "${1}" == "install" -a "${NUM}" != "0" ]; then
    # grab the status of the existing chart release
    debuglog_helm_status "${NS}"  "${RELEASE}"
    log error "The Module is already installed"
  fi
  if [ "${1}" == "upgrade" -a "${NUM}" == "0" ]; then
    log error "The Module is not installed"
  fi
}

#
# validate_params will validate the parameters passed in
function validate_params() {
  # make sure the driver was specified
  if [ -z "${DRIVER}" ]; then
    decho "No driver specified"
    usage
    exit 1
  fi
  # the namespace is required
  if [ -z "${NS}" ]; then
    decho "No namespace specified"
    usage
    exit 1
  fi
  # values file
  if [ -z "${VALUES}" ]; then
    decho "No values file was specified"
    usage
    exit 1
  fi
  if [ ! -f "${VALUES}" ]; then
    decho "Unable to read values file at: ${VALUES}"
    usage
    exit 1
  fi
}

#
# install_driver uses helm to install the driver with a given name
function install_driver() {
  if [ "${1}" == "upgrade" ]; then
    log step "Upgrading Driver"
  else
    log step "Installing Driver"
  fi

  # run driver specific install script
  local SCRIPTNAME="install-${DRIVER}.sh"
  if [ -f "${SCRIPTDIR}/${SCRIPTNAME}" ]; then
    source "${SCRIPTDIR}/${SCRIPTNAME}"
  fi

  HELMOUTPUT="/tmp/csi-install.$$.out"
  run_command helm ${1} \
    --set openshift=${OPENSHIFT} \
    --values "${VALUES}" \
    --namespace ${NS} "${RELEASE}" \
    "${DRIVERDIR}/${DRIVER}" >"${HELMOUTPUT}" 2>&1

  if [ $? -ne 0 ]; then
    cat "${HELMOUTPUT}"
    log error "Helm operation failed, output can be found in ${HELMOUTPUT}. The failure should be examined, before proceeding. Additionally, running csi-uninstall.sh may be needed to clean up partial deployments."
  fi
  log step_success
  if [ $? -eq 1 ]; then
    warning "Timed out waiting for the operation to complete." \
      "This does not indicate a fatal error, pods may take a while to start." \
      "Progress can be checked by running \"kubectl get pods -n ${NS}\""
    debuglog_helm_status "${NS}" "${RELEASE}"
  fi
}

# Print a nice summary at the end
function summary() {
  log section "Operation complete"
}


function kubectl_safe() {
  eval "kubectl $1"
  exitcode=$?
  if [[ $exitcode != 0 ]]; then
    decho "$2"
    decho "Command was: kubectl $1"
    decho "Output was:"
    eval "kubectl $1"
    exit $exitcode
  fi
}
function found_error() {
  for N in "$@"; do
    ERRORS+=("${N}")
  done
}
# verify namespace
function verify_namespace() {
  log step "Verifying that required namespaces have been created"

  error=0
  for N in "${@}"; do
    # Make sure the namespace exists
    run_command kubectl describe namespace "${N}" >/dev/null 2>&1
    if [ $? -ne 0 ]; then
      log error "Namespace does not exist: ${N}"
    fi
    log passed
  done
}


#
# verify_kubernetes
# will run a driver specific function to verify environmental requirements
function verify_kubernetes() {
 verify_namespace $NS
}

#
# main
#
VERIFYOPTS=""
ASSUMEYES="false"

# get the list of valid CSI Drivers, this will be the list of directories in drivers/ that contain helm charts
get_drivers "${DRIVERDIR}"
# if only one driver was found, set the DRIVER to that one
if [ ${#VALIDDRIVERS[@]} -eq 1 ]; then
  DRIVER="${VALIDDRIVERS[0]}"
fi

while getopts ":h-:" optchar; do
  case "${optchar}" in
  -)
    case "${OPTARG}" in
    upgrade)
      MODE="upgrade"
      ;;
      # NAMESPACE
    namespace)
      NS="${!OPTIND}"
      if [[ -z ${NS} || ${NS} == "--skip-verify" ]]; then
        NS=${DEFAULT_NS}
      else
        OPTIND=$((OPTIND + 1))
      fi
      ;;
    namespace=*)
      NS=${OPTARG#*=}
      if [[ -z ${NS} ]]; then NS=${DEFAULT_NS}; fi
      ;;
      # RELEASE
    release)
      RELEASE="${!OPTIND}"
      OPTIND=$((OPTIND + 1))
      ;;
    release=*)
      RELEASE=${OPTARG#*=}
      ;;
      # VALUES
    values)
      VALUES="${!OPTIND}"
      OPTIND=$((OPTIND + 1))
      ;;
    values=*)
      VALUES=${OPTARG#*=}
      ;;
    *)
      decho "Unknown option --${OPTARG}"
      decho "For help, run $PROG -h"
      exit 1
      ;;
    esac
    ;;
  h)
    usage
    ;;
  *)
    decho "Unknown option -${OPTARG}"
    decho "For help, run $PROG -h"
    exit 1
    ;;
  esac
done

# by default the NAME of the helm release of the driver is the same as the driver name
RELEASE=$(get_release_name "${DRIVER}")
# by default, NODEUSER is root
NODEUSER="${NODEUSER:-root}"

# make sure kubectl is available
kubectl --help >&/dev/null || {
  decho "kubectl required for installation... exiting"
  exit 2
}
# make sure helm is available
helm --help >&/dev/null || {
  decho "helm required for installation... exiting"
  exit 2
}

OPENSHIFT=$(isOpenShift)

# Get the kubernetes major and minor version numbers.
kMajorVersion=$(run_command kubectl version | grep 'Server Version' | sed -e 's/^.*Major:"//' -e 's/[^0-9].*//g')
kMinorVersion=$(run_command kubectl version | grep 'Server Version' | sed -e 's/^.*Minor:"//' -e 's/[^0-9].*//g')

# validate the parameters passed in
validate_params "${MODE}"

header
check_for_driver "${MODE}"
verify_kubernetes

# all good, keep processing
install_driver "${MODE}"

summary
