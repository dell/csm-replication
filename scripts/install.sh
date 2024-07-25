#!/bin/bash
#
######################################################################
# Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.
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

SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
MODULEDIR="${SCRIPTDIR}/../"
PROG="${0}"
MODE="install"
NS="dell-replication-controller"
RELEASE="replication"
MODULE="csm-replication"
HELMCHARTVERSION="csm-replication-1.9.0"

# export the name of the debug log, so child processes will see it
export DEBUGLOG="${SCRIPTDIR}/install-debug.log"

source "$SCRIPTDIR"/common.sh

if [ -f "${DEBUGLOG}" ]; then
  rm -f "${DEBUGLOG}"
fi

# usage will print command execution help and then exit
function usage() {
  decho
  decho "Help for $PROG"
  decho
  decho "Usage: $PROG options..."
  decho "Options:"
  decho "  Required"
  decho "  --values[=]<values.yaml>                 Values file, which defines configuration values"

  decho "  Optional"
  decho "  --upgrade                                Perform an upgrade, default is false"
  decho "  --helm-charts-version                    Pass the helm chart version"
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
  if [ "${MODE}" == "upgrade" ]; then
    log section "Upgrading ${MODULE} module on ${kMajorVersion}.${kMinorVersion}"
  else
    log section "Installing ${MODULE} module on ${kMajorVersion}.${kMinorVersion}"
  fi
}

# check_for_module will see if the module is already installed within the namespace provided
function check_for_module() {
  log step "Checking to see if module is already installed"
  NUM=$(run_command helm list --namespace "${NS}" | grep "^${RELEASE}\b" | wc -l)
  if [ "${1}" == "install" -a "${NUM}" != "0" ]; then
    # grab the status of the existing chart release
    debuglog_helm_status "${NS}" "${RELEASE}"
    log step_failure
    log error "The ${MODULE} module is already installed"
  fi
  if [ "${1}" == "upgrade" -a "${NUM}" == "0" ]; then
    log step_failure
    log error "The ${MODULE} module is not installed"
  fi
  log step_success
}

# validate_params will validate the parameters passed in
function validate_params() {
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

# install_module uses helm to install the module with a given name
function install_module() {
  if [ "${1}" == "upgrade" ]; then
    log step "Upgrading ${MODULE} module"
  else
    log step "Installing ${MODULE} module"
  fi

  HELMOUTPUT="/tmp/csm-install.$$.out"
  run_command helm ${1} \
    --set openshift=${OPENSHIFT} \
    --values "${VALUES}" \
    --namespace ${NS} "${RELEASE}" \
    "${MODULEDIR}/${MODULE}" >"${HELMOUTPUT}" 2>&1

  if [ $? -ne 0 ]; then
    cat "${HELMOUTPUT}"
    log error "Helm operation failed, output can be found in ${HELMOUTPUT}. The failure should be examined, before proceeding."
  fi
  log step_success
  # wait for the deployment to finish, use the default timeout
  waitOnRunning "${NS}" "Deployment dell-replication-controller-manager"
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
      log step_failure
      log error "Namespace does not exist: ${N}"
    fi
    log passed
  done
}

# waitOnRunning
# will wait, for a timeout period, for a number of pods to go into Running state within a namespace
# arguments:
#  $1: required: namespace to watch
#  $2: required: comma seperated list of deployment type and name pairs
#      for example: "statefulset mystatefulset,daemonset mydaemonset"
#  $3: optional: timeout value, 300 seconds is the default.
function waitOnRunning() {
  if [ -z "${2}" ]; then
    decho "No namespace and/or list of deployments was supplied. This field is required for waitOnRunning"
    return 1
  fi
  # namespace
  local NS="${1}"
  # pods
  IFS="," read -r -a PODS <<<"${2}"
  # timeout value passed in, or 300 seconds as a default
  local TIMEOUT="300"
  if [ -n "${3}" ]; then
    TIMEOUT="${3}"
  fi

  error=0
  for D in "${PODS[@]}"; do
    log arrow
    log smart_step "Waiting for Deployment to be ready" "small"
    run_command kubectl -n "${NS}" rollout status --timeout=${TIMEOUT}s ${D} >/dev/null 2>&1
    if [ $? -ne 0 ]; then
      error=1
      log step_failure
    else
      log step_success
    fi
  done

  if [ $error -ne 0 ]; then
    return 1
  fi
  return 0
}

# verify_kubernetes
# will run a module specific function to verify environmental requirements
function verify_kubernetes() {
  verify_namespace $NS
}

#
# main
#

ASSUMEYES="false"

while getopts ":h-:" optchar; do
  case "${optchar}" in
  -)
    case "${OPTARG}" in
    upgrade)
      MODE="upgrade"
      ;;
    helm-charts-version)
      HELMCHARTVERSION="${!OPTIND}"
      OPTIND=$((OPTIND + 1))
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

if [ ! -d "$MODULEDIR/helm-charts" ]; then
  if  [ ! -d "$SCRIPTDIR/helm-charts" ]; then
    git clone --quiet -c advice.detachedHead=false -b $HELMCHARTVERSION https://github.com/dell/helm-charts
  fi
  mv helm-charts $MODULEDIR
else
  if [  -d "$SCRIPTDIR/helm-charts" ]; then
    rm -rf $SCRIPTDIR/helm-charts
  fi
fi
MODULEDIR="${SCRIPTDIR}/../helm-charts/charts"

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
check_for_module "${MODE}"
verify_kubernetes

# all good, keep processing
install_module "${MODE}"

summary
