#!/bin/bash

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

#This file is a placeholder to satisfy GoSec scans. It will be replaced by one in csm-replication/scripts upon compilation using makefile.
=======
#!/bin/bash

#######################################
# Prints formatted command's action
# Arguments:
#   $1 - custom message
#######################################
function t_print_action() {
  printf "%-50s" "$1"
}

#######################################
# Prints formatted command's status
# Globals:
#   green - predefined color variable
#   nc - predefined color variable
# Arguments:
#   $1 - custom message
#######################################
function t_print_status() {
  if [ -z "${1}" ]; then
    printf "%-55s\n" "${green}[  OK  ]${nc}"
  else
    printf "%-55s\n" "$1"
  fi
}

#######################################
# Prints error statement
# Globals:
#   nc - predefined color variable
#   red - predefined color variable
# Arguments:
#  None
#######################################
function finish_error() {
  echo -e "${red}[ FAIL ]${nc}"
  exit
}

#######################################
# Generates kubeconfig for service account in k8s.
# Globals:
#   gncert - CA's certificate
#   gnsecret - SA's secret
#   gnserver - server's URL
#   gntoken - SA's token
#   ns - (optional) SA'a namespace
#   output - output file location
#   o - output file location (CLI arg)
#   s - service account name (CLI arg)
# Arguments:
#  None
#######################################
function service_branch() {
  if [ -z "${o}" ]; then
    output="./configs/config-$s"
  fi
  t_print_action "Compiling kubeconfig for \"$s\"" && t_print_status
  t_print_action "Extracting secret"
  gnsecret=$(kubectl -n $ns describe sa $s 2>/dev/null | grep Mountable | awk '{print $3}') && t_print_status || finish_error
  t_print_action "Extracting token"
  gntoken=$(kubectl -n $ns describe secret $gnsecret | grep token: | awk '{print $2}') && t_print_status || finish_error
  t_print_action "Extracting CA"
  gncert=$(kubectl config view --flatten --minify | grep certificate-authority-data: | awk '{print $2}') && t_print_status || finish_error
  t_print_action "Extracting server URL"
  gnserver=$(kubectl config view --flatten --minify | grep server: | awk '{print $2}') && t_print_status || finish_error

  export gnsecret
  export gntoken
  export gncert
  export gnserver

  folder=$(dirname "$0")
  envsubst >"$output" < "$folder"/config-placeholder
  echo "DONE. Kubeconfig location: $output"

}

#######################################
# Generates kubeconfig for "normal" user in k8s.
# Globals:
#   base64_csr - base64 encoded CSR
#   certname - CN to use for kubeconfig creation
#   output - output file location
#   c - Certificate Signing Request
#   k - user's private key
#   o - output file location
#   u - user's username
# Arguments:
#  None
#######################################
function user_branch() {
  if [ -z "${c}" ] || [ -z "${k}" ]; then
    usage
  fi
  if [ -z "${o}" ]; then
    output="./configs/config-$u"
  fi
  t_print_action "Encoding and applying CSR.. "
  export base64_csr=$(cat $c | base64 | tr -d '\n')
  export certname=$u
  cat csr.yaml | envsubst | kubectl apply -f - &>/dev/null && t_print_status || finish_error

  t_print_action "Approving CSR.. "
  kubectl certificate approve $u &>/dev/null && t_print_status || finish_error

  t_print_action "CSR Status: "
  t_print_status "$(kubectl get csr | grep $u | awk '{print $5}')"

  t_print_action "Saving cert.. "
  kubectl get csr $u -o jsonpath='{.status.certificate}' | base64 --decode >./tmp/$u.crt && t_print_status || finish_error

  t_print_action "Using name \"$u\".. "
  t_print_status

  t_print_action "Saving temp CA cert.. "
  kubectl config view -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' --raw | base64 --decode - >./tmp/kubernetes-ca.crt && t_print_status || finish_error

  t_print_action "Creating kubeconfig for \"$u\".. "
  kubectl config set-cluster $(kubectl config view -o jsonpath='{.clusters[0].name}') --server=$(kubectl config view -o jsonpath='{.clusters[0].cluster.server}') --certificate-authority=./tmp/kubernetes-ca.crt --kubeconfig=$output --embed-certs &>/dev/null && t_print_status || finish_error

  t_print_action "Setting credentials.. "
  kubectl config set-credentials $u --client-certificate=./tmp/$u.crt --client-key=$k --embed-certs --kubeconfig=$output &>/dev/null && t_print_status || finish_error

  t_print_action "Setting context.. "
  kubectl config set-context $u --cluster=$(kubectl config view -o jsonpath='{.clusters[0].name}') --namespace=default --user=$u --kubeconfig=$output &>/dev/null && t_print_status || finish_error

  t_print_action "Applying context.. "
  kubectl config use-context $u --kubeconfig=$output &>/dev/null && t_print_status || finish_error
  echo "DONE. Kubeconfig location: $output"
}

#######################################
# Prints usage information and exits
#######################################
function usage() {
  echo "Usage: $0 (( -u <CN user> -c <CSR> -k <key> || -s <serviceAccount> [-n <namespace>] )) && [-o <path/to/outputfile>]" 1>&2
  exit 1
}

#######################################
# Main entrypoint
# Arguments:
#  None
#######################################
function main() {
  set -o pipefail

  ns=default

  red=$(tput setaf 1)

  green=$(tput setaf 2)

  nc=$(tput sgr0)

  mkdir -p ./tmp

  mkdir -p ./configs

  while getopts ":u:s:o:n:c:k:" i; do
    case "${i}" in
      u)
        u=${OPTARG}
        ;;
      c)
        c=${OPTARG}
        ;;
      k)
        k=${OPTARG}
        ;;
      s)
        s=${OPTARG}
        ;;
      n)
        n=${OPTARG}
        ns=$n
        ;;
      o)
        o=${OPTARG}
        output="$o"
        ;;
      *)
        usage
        ;;
    esac
  done

  shift $((OPTIND - 1))

  if [ -z "${u}" ] && [ -z "${s}" ]; then
    usage
  fi

  if [ -z "${u}" ]; then
    service_branch
  else
    user_branch
  fi

  rm -rf ./tmp

}

main "$@"