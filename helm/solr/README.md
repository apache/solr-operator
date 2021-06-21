Apache Solr
=============

Apache Solr is the popular, blazing-fast, open source enterprise search platform built on Apache Lucene™.

Solr is highly reliable, scalable and fault tolerant, providing distributed indexing, replication and load-balanced querying, automated failover and recovery, centralized configuration and more.
Solr powers the search and navigation features of many of the world's largest internet sites.

Documentation around using the Solr Operator can be found at it's [official site](https://solr.apache.org).

## Dependencies

This Helm chart relies on the [Solr Operator](https://solr.apache.org/operator) being installed.
You can install the Solr Operator via it's [Helm chart](https://artifacthub.io/packages/helm/apache-solr/solr-operator).

If you are using a provided Zookeeper instance for your Solr Cloud, then you must also install the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator).
However, it is [installed by default as a dependency](https://artifacthub.io/packages/helm/apache-solr/solr-operator#installing-the-zookeeper-operator) when using the Solr Operator Helm chart.

## Using the Helm Chart

### Installing the Chart

To install the Solr Operator for the first time in your cluster, you can use the latest version or a specific version, run with the following commands:

```bash
kubectl create -f https://solr.apache.org/operator/downloads/crds/v0.4.0-prerelease/all-with-dependencies.yaml
helm install solr-operator apache-solr/solr-operator --version 0.4.0-prerelease
```

The command deploys the solr-operator on the Kubernetes cluster with the default configuration.
The [configuration](#chart-values) section lists the parameters that can be configured during installation.

_Note that the Helm chart version does not contain a `v` prefix, which the downloads version does. The Helm chart version is the only part of the Solr Operator release that does not use the `v` prefix._

### Upgrading the Solr Operator

If you are upgrading your Solr Operator deployment, you should always use a specific version of the chart and pre-install the Solr CRDS:

```bash
kubectl replace -f https://solr.apache.org/operator/downloads/crds/v0.4.0-prerelease/all-with-dependencies.yaml
helm upgrade solr-operator apache-solr/solr-operator --version 0.4.0-prerelease
```

#### The ZookeeperCluster CRD

If you use the provided Zookeeper Cluster in the SolrCloud Spec, it is important to make sure you have the correct `ZookeeperCluster` CRD installed as well.

The Zookeeper Operator Helm chart includes its CRDs when installing, however the way the CRDs are managed can be considered risky.
If we let the Zookeeper Operator Helm chart manage the Zookeeper CRDs, then users could see outages when [uninstalling the chart](#uninstalling-the-chart).
Therefore, by default, we tell the Zookeeper Operator to not install the Zookeeper CRDs.
You can override this, assuming the risks, by setting `zookeeper-operator.crd.create: true`.

For manual installation of the ZookeeperCluster CRD, you can find the file in the [zookeeper-operator repo](https://github.com/pravega/zookeeper-operator/blob/master/deploy/crds/zookeeper.pravega.io_zookeeperclusters_crd.yaml), for the correct release,
or use the convenience download locations provided below.
The Solr CRD releases have bundled the ZookeeperCluster CRD required in each version.

```bash
# Install all Solr CRDs as well as the dependency CRDS (ZookeeperCluster) for the given version of the Solr Operator
kubectl create -f "https://solr.apache.org/operator/downloads/crds/<solr operator version>/all-with-dependencies.yaml"

# Install *just* the ZookeeperCluster CRD used in the given version of the Solr Operator
kubectl create -f "https://solr.apache.org/operator/downloads/crds/<solr operator version>/zookeeperclusters.yaml"
```

Examples:
- `https://solr.apache.org/operator/downloads/crds/v0.3.0/all-with-dependencies.yaml`  
  Includes all Solr CRDs and dependency CRDs, including `ZookeeperCluster`, in the `v0.3.0` Solr Operator release
- `https://solr.apache.org/operator/downloads/crds/v0.2.8/zookeeperclusters.yaml`  
  Just the ZookeeperCluster CRD required in the `v0.2.8` Solr Operator release

### Uninstalling the Chart

To uninstall/delete the Solr Operator, run:

```bash
helm uninstall solr-operator
```

The command removes all the Kubernetes components associated with the chart and deletes the release, except for the Solr CRDs.

**NOTE: If you are using the Zookeeper Operator helm chart as a dependency and have set `zookeeper-operator.crds.install: true`, this will also delete the ZookeeperCluster CRD, thus deleting all Zookeeper instances in the Kubernetes Cluster.**


## Chart Values

### Running Solr

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| fullnameOverride | string | `""` | A custom name for the Solr Operator Deployment |
| nameOverride | string | `""` |  |
| replicaCount | int | `3` | The number of Solr pods to run in the Solr Cloud. |
| image.repository | string | `apache/solr` | The repository of the Solr image |
| image.tag | string | `8.9` | The tag/version of Solr to run |
| image.pullPolicy | string |  | PullPolicy for the Solr image, defaults to the empty Pod behavior |
| image.imagePullSecret | string |  | PullSecret for the Solr image |
| busyBoxImage.repository | string | `apache/solr` | The repository of the BusyBox image |
| busyBoxImage.tag | string | `8.9` | The tag/version of BusyBox to run |
| busyBoxImage.pullPolicy | string |  | PullPolicy for the BusyBox image, defaults to the empty Pod behavior |
| busyBoxImage.imagePullSecret | string |  | PullSecret for the BusyBox image |
| solrOptions.memory | string | `"-Xms1g -Xmx2g"` | PullSecret for the BusyBox image |
| solrOptions.javaOpts | string | `""` | Additional java arguments to pass via the command line |
| solrOptions.logLevel | string | `"INFO"` | Log level to run Solr under |
| solrOptions.gcTune | string | `""` | GC Tuning parameters for Solr |
| solrOptions.security.authenticationType | string | `""` | Type of authentication to use for Solr |
| solrOptions.security.basicAuthSecret | string | `""` | Name of Secret in the same namespace that stores the basicAuth information for the Solr user |
| solrOptions.security.probesRequireAuth | boolean | | Whether the probes for the SolrCloud pod require auth |
| solrTLS.pkcs12Secret.name | string |  |  |

### Global Options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| global.imagePullSecrets | object[] |  | The list of imagePullSecrets to include in pods |
| global.clusterDomain | string |  | The cluster domain the Kubernetes is addressed under. |

### Custom Kubernetes Options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| podOptions.annotations | map[string]string |  | Custom annotations to add to the Solr pod |
| podOptions.labels | map[string]string |  | Custom labels to add to the Solr pod |
| podOptions.resources.limits | map[string]string |  | Provide Resource limits for the Solr container |
| podOptions.resources.requests | map[string]string |  | Provide Resource requests for the Solr container |
| podOptions.nodeSelector | map[string]string |  | Add a node selector for the Solr pod, to specify where it can be scheduled |
| podOptions.affinity | object |  | Add Kubernetes affinity information for the Solr pod |
| podOptions.tolerations | []object |  | Specify a list of Kubernetes tolerations for the Solr pod |
| podOptions.priorityClassName | string | | Give a priorityClassName for the Solr pod |
| podOptions.sidecarContainers | []object |  | An optional list of additional containers to run along side the Solr in its pod |
| podOptions.initContainers | []object |  | An optional list of additional initContainers to run before the Solr container starts |
| podOptions.envVars | []object |  | List of additional environment variables for the Solr container |
| podOptions.podSecurityContext | object |  | Security context for the Solr pod |
| podOptions.terminationGracePeriodSeconds | int |  | Optional amount of time to wait for Solr to stop on its own, before manually killing it |
| podOptions.livenessProbe | object |  | Custom liveness probe for the Solr container |
| podOptions.readinessProbe | object |  | Custom readiness probe for the Solr container |
| podOptions.startupProbe | object |  | Custom startup probe for the Solr container |
| podOptions.imagePullSecrets | []object |  | List of image pull secrets to inject into the Solr pod, in addition to `global.imagePullSecrets` |
| podOptions.volumes | []object |  | List of additional volumes to attach to the Solr pod, and optionally how to mount them to the Solr container |
| statefulSetOptions.annotations | map[string]string |  | Custom annotations to add to the Solr statefulSet |
| statefulSetOptions.labels | map[string]string |  | Custom labels to add to the Solr statefulSet |
| statefulSetOptions.podManagementPolicy | string | "Parallel" | Policy for how Solr pods should be managed in the statefulSet, "OrderedReady" or "Parallel". |
| commonServiceOptions.annotations | map[string]string |  | Custom annotations to add to the Solr common service |
| commonServiceOptions.labels | map[string]string |  | Custom labels to add to the Solr common service |
| headlessServiceOptions.annotations | map[string]string |  | Custom annotations to add to the Solr headless service |
| headlessServiceOptions.labels | map[string]string |  | Custom labels to add to the Solr headless service |
| nodeServiceOptions.annotations | map[string]string |  | Custom annotations to add to the Solr node service(s) |
| nodeServiceOptions.labels | map[string]string |  | Custom labels to add to the Solr node service(s) |
| ingressOptions.annotations | map[string]string |  | Custom annotations to add to the Solr ingress, if it exists |
| ingressOptions.labels | map[string]string |  | Custom labels to add to the Solr ingress, if it exists |
| configMapOptions.annotations | map[string]string |  | Custom annotations to add to the Solr configMap |
| configMapOptions.labels | map[string]string |  | Custom labels to add to the Solr configMap |
| configMapOptions.providedConfigMap | string |  | Provide an existing configMap for the Solr XML and/or Solr log4j files. *ADVANCED* |
