# OCI Registry Support for CSM Replication

## Overview

Dell Container Storage Modules (CSM) Replication supports installation and upgrade from OCI-compliant registries. This allows you to store and distribute the CSM Replication Helm chart through container registries instead of using local Helm charts or traditional Helm chart repositories.

## Prerequisites

Before installing CSM Replication from an OCI registry, ensure you have:

1. **Helm v3.8+** or **Helm v4** installed
2. **kubectl** configured with access to your Kubernetes cluster
3. **OCI registry** with the CSM Replication Helm chart published
4. **Registry credentials** (if the registry requires authentication)

## Installing from OCI Registry

### Step 1: Create the Namespace

Create the namespace where CSM Replication will be installed:

```bash
kubectl create namespace dell-replication-controller
```

### Step 2: Create Registry Credentials Secret (if required)

If your OCI registry requires authentication, create a Kubernetes secret containing your registry credentials:

```bash
kubectl create secret generic oci-registry-creds \
  --from-literal=username=<your-username> \
  --from-literal=password=<your-password> \
  --namespace dell-replication-controller
```

**Note:** The secret must contain two keys: `username` and `password`. These will be base64-encoded automatically by Kubernetes.

### Step 3: Prepare Your Values File

Download and customize the values file for CSM Replication:

```bash
# Download the template values file
wget -O my-replication-values.yaml https://raw.githubusercontent.com/dell/helm-charts/main/charts/csm-replication/values.yaml

# Edit the values file with your configuration
vi my-replication-values.yaml
```

### Step 4: Install from OCI Registry

Run the installation script with the OCI registry parameters:

```bash
cd csm-replication/scripts

./install.sh \
  --values my-replication-values.yaml \
  --oci-chart oci://registry.example.com/charts/csm-replication \
  --registry-auth-secret oci-registry-creds \
  --helm-charts-version 1.16.0
```

**Parameters:**
- `--values`: Path to your customized values file
- `--oci-chart`: OCI registry URI for the Helm chart (must start with `oci://`)
- `--registry-auth-secret`: Name of the Kubernetes secret containing registry credentials
- `--helm-charts-version`: Version of the Helm chart to install (e.g., `1.16.0`)

### Step 5: Verify Installation

Check that the CSM Replication controller is running:

```bash
kubectl get pods -n dell-replication-controller
```

You should see the `dell-replication-controller-manager` pod in `Running` state.

## Upgrading from OCI Registry

To upgrade CSM Replication from an OCI registry, use the same `install.sh` script with the `--upgrade` flag:

```bash
./install.sh \
  --values my-replication-values.yaml \
  --upgrade \
  --oci-chart oci://registry.example.com/charts/csm-replication \
  --registry-auth-secret oci-registry-creds \
  --helm-charts-version 1.17.0
```

## Installing Without Authentication

If your OCI registry does not require authentication (e.g., a public registry or internal registry with no auth), you can omit the `--registry-auth-secret` parameter:

```bash
./install.sh \
  --values my-replication-values.yaml \
  --oci-chart oci://registry.example.com/charts/csm-replication \
  --helm-charts-version 1.16.0
```

A warning will be displayed indicating that registry login will be skipped.

## Helm v4 Compatibility

CSM Replication installation script is fully compatible with both Helm v3 and Helm v4. The script automatically detects the installed Helm version and validates compatibility.

### Helm v4 Features

When using Helm v4, the installation benefits from:
- **Server-Side Apply (SSA)**: Improved handling of resource ownership and field management
- **Enhanced OCI support**: Native OCI registry operations with improved performance
- **Better conflict resolution**: Automatic handling of field ownership conflicts

### Helm Version Detection

The installation script automatically:
1. Detects the installed Helm version
2. Validates that Helm v3.8+ or Helm v4 is installed
3. Adjusts behavior based on the detected version

## OCI Registry URI Format

The OCI chart URI must follow this format:

```
oci://<registry-domain>/<path>/<chart-name>
```

**Examples:**
- `oci://registry.example.com/charts/csm-replication`
- `oci://harbor.company.com/dell/csm-replication`
- `oci://localhost:5000/charts/csm-replication` (for local testing)

**Note:** For localhost registries (`localhost` or `127.0.0.1`), the script automatically adds the `--plain-http` flag to support non-HTTPS connections.

## Usage Information

For complete usage information, run:

```bash
./install.sh -h
```

**Output:**
```
Help for ./install.sh

Usage: ./install.sh options...
Options:
  Required
  --values[=]<values.yaml>                 Values file, which defines configuration values

  Optional
  --upgrade                                Perform an upgrade, default is false
  --helm-charts-version                    Helm chart version (format: version number for OCI Registry e.g. 1.16.0)
  --oci-chart[=]<oci-uri>                  OCI registry URI for Helm chart (e.g., oci://registry.example.com/charts/csm-replication)
  --registry-auth-secret[=]<secret-name>   Kubernetes secret containing registry credentials (username/password keys)
  -h                                       Help
```

## Troubleshooting

### Authentication Failures

If you encounter authentication errors:

1. Verify the secret exists in the correct namespace:
   ```bash
   kubectl get secret oci-registry-creds -n dell-replication-controller
   ```

2. Check the secret contains the correct keys:
   ```bash
   kubectl get secret oci-registry-creds -n dell-replication-controller -o yaml
   ```

3. Ensure the credentials are correct by testing manual login:
   ```bash
   helm registry login registry.example.com -u <username> -p <password>
   ```

### Chart Not Found

If the chart cannot be found:

1. Verify the OCI URI is correct
2. Check that the chart version exists in the registry
3. Ensure you have access to the registry and chart

### Version Mismatch

If you see version-related errors:

1. Verify the `--helm-charts-version` matches an available chart version
2. Check the registry for available versions
3. Ensure the version format is correct (e.g., `1.16.0`, not `v1.16.0`)

