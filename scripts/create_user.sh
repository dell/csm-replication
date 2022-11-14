#!/bin/bash
######################################################################
# Copyright © 2021 Dell Inc. or its subsidiaries. All Rights Reserved.
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

set -e

# Helper script to create a "User" in Kubernetes
# It does the following 
# 1. Create a private key(RSA) for the user
# 2. Create a Certificate Signing Request(CSR)
# 3. Create a CertificateSigningRequest object in Kubernetes with the CSR
# 4. Approve the CertificateSigningRequest using kubectl
# 5. Retrieve the signed certificate for the user
# 6. Create a kubeconfig for the newly created user using the provided server IP and the CA certificates

# First check if username has been specified via env
if [ -z ${KUBE_USER+x} ]; then
  echo "KUBE_USER env is not set. Please provide a username"
  exit 1
fi

# Check if server IP has been provided
if [ -z ${SERVER+x} ]; then
  echo "SERVER IP(& Port) has not been provided. Please set the SERVER env variable"
  exit 1
fi

# Check if cluster's CA certificate path has been provided
if [ -z ${SERVER_CA+x} ]; then
  echo "Warning: SERVER_CA path is unset.Defaulting to /etc/kubernetes/pki/ca.crt"
  SERVER_CA="/etc/kubernetes/pki/ca.crt"
fi


# Check if target directory already exists
TARGET_DIR=.$KUBE_USER

echo "$TARGET_DIR will be used to store all artifacts"

if [ -d "$TARGET_DIR" ]; then
  echo "$TARGET_DIR already exists. Delete it and rerun the script"
  exit 1
fi

mkdir -p $TARGET_DIR

# Check if the CA file exists
if [ -f "$SERVER_CA" ]; then
  echo "Verified that $SERVER_CA exists."
else
  echo "$SERVER_CA not found. Exiting with error"
  exit 1
fi

PRIVATE_KEY="$KUBE_USER.key"
CSR_FILE="$KUBE_USER.csr"
CSR_YAML="$CSR_FILE.yaml"
CLIENT_CERT="$KUBE_USER.crt"

COUNTRY="US"
STATE="Massachusetts"
LOCALITY="Hopkinton"
ORGANISATION="Dell"
COMMONNAME="$KUBE_USER"

# Create a key for the user
echo "openssl genrsa -out $TARGET_DIR/$PRIVATE_KEY 2048"
openssl genrsa -out $TARGET_DIR/$PRIVATE_KEY 2048

# Create a SSL config

cat <<EOF > .temp-openssl-config
[ req ]
default_bits           = 2048
distinguished_name     = req_distinguished_name
prompt                 = no
encrypt_key            = no
string_mask            = utf8only
req_extensions         = v3_req
[ req_distinguished_name ]
C                      = ${COUNTRY}
ST                     = ${STATE}
L                      = ${LOCALITY}
O                      = ${ORGANISATION}
CN                     = ${COMMONNAME}
[ v3_req ]
basicConstraints       = CA:FALSE
keyUsage               = nonRepudiation, digitalSignature, keyEncipherment
EOF


# Create a CSR
echo "openssl req -config .temp-openssl-config -new -key $TARGET_DIR/$PRIVATE_KEY -out $TARGET_DIR/$CSR_FILE"
openssl req -config .temp-openssl-config -new -key $TARGET_DIR/$PRIVATE_KEY -out $TARGET_DIR/$CSR_FILE

# Delete the openssl config
echo rm -f .temp-openssl-config
rm -f .temp-openssl-config

BASE64_ENCODED_CSR=$(cat $TARGET_DIR/$CSR_FILE | base64 | tr -d "\n")

# Next create a signing request yaml from the apiserver signing authority
cat <<EOF>> $TARGET_DIR/$CSR_YAML
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: $KUBE_USER
spec:
  groups:
  - system:authenticated
  request: $BASE64_ENCODED_CSR
  signerName: kubernetes.io/kube-apiserver-client
  usages:
  - client auth
EOF

# Create the signing request
kubectl create -f $TARGET_DIR/$CSR_YAML

# Display the CSR request
kubectl get csr

# Approve the CSR
kubectl certificate approve $KUBE_USER

echo "Sleeping for 5 seconds to allow Kubernetes to update the object"
sleep 5

# Display the CSR request again
echo
kubectl get csr
echo

# Create the certificate file using the status field of the CSR object
echo "Creating the user certificate file"
echo "kubectl get csr $KUBE_USER -o json | jq -r '.status.certificate' | base64 --decode > $TARGET_DIR/$CLIENT_CERT"
kubectl get csr $KUBE_USER -o json | jq -r '.status.certificate' | base64 --decode > $TARGET_DIR/$CLIENT_CERT

CONFIG_FILE=$KUBE_USER.conf
# Now create the kubeconfig file

# First embed the cluster info
kubectl config --kubeconfig=$TARGET_DIR/$CONFIG_FILE set-cluster kubernetes --server=$SERVER --certificate-authority=$SERVER_CA --embed-certs=true

# User details
kubectl config --kubeconfig=$TARGET_DIR/$CONFIG_FILE set-credentials $KUBE_USER --client-key=$TARGET_DIR/$PRIVATE_KEY --client-certificate=$TARGET_DIR/$CLIENT_CERT --embed-certs

# Create context
kubectl config --kubeconfig=$TARGET_DIR/$CONFIG_FILE set-context $KUBE_USER --cluster=kubernetes --namespace=default --user=$KUBE_USER

# Set context
kubectl config --kubeconfig=$TARGET_DIR/$CONFIG_FILE use-context $KUBE_USER

echo
echo "The following files were created"
echo "#####################################################################################"
echo "Private key: $TARGET_DIR/$PRIVATE_KEY"
echo "CSR: $TARGET_DIR/$CSR_FILE"
echo "CSR Yaml: $TARGET_DIR/$CSR_YAML"
echo "User Certificate: $TARGET_DIR/$CLIENT_CERT"
echo "kubeconfig for the user: $TARGET_DIR/$CONFIG_FILE"
echo "#####################################################################################"
