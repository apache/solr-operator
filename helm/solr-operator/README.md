Solr Operator
=============

The Solr Operator is designed to allow easy deployment Solr Clouds and other Solr Resources to Kubernetes.

Documentation around using the Solr Operator can be found at it's [official site](https://solr.apache.org/operator) or [source repo](https://github.com/apache/solr-operator).

[Tutorials](https://solr.apache.org/operator/resources.html#tutorials) have been provided for both basic and advanced usage of the Solr Operator.

## Upgrade Notes

Before upgrading your Solr Operator to a newer version, **please refer to the [Upgrade Notes](https://apache.github.io/solr-operator/docs/upgrade-notes.html)**.
There may be breaking changes between the version you are running and the version you want to upgrade to.

## Using the Helm Chart

### Installing the Zookeeper Operator

Before installing the Solr Operator, we need to install the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator).
This is because the Solr Operator, in most instances, relies on the Zookeeper Operator to create the Zookeeper clusters that Solr coordinates through.

The Solr Operator helm chart has a conditional dependency on the [official Zookeeper Operator helm chart](https://github.com/pravega/zookeeper-operator/tree/v0.2.12/charts/zookeeper-operator#installing-the-chart),
which is enabled **by default**.

If you wish to manage the installation of the Zookeeper Operator yourself, set:
- `zookeeper-operator.install: false`
- `zookeeper-operator.use: true`

If you do not wish to use the Zookeeper Operator, set:
- `zookeeper-operator.install: false`
- `zookeeper-operator.use: false`


### Adding the Solr Operator Helm Chart Repository
You should only need to add the solr operator helm chart repository once, by running the following command:

```bash
helm repo add apache-solr https://solr.apache.org/charts
```

### Installing the Chart

To install the Solr Operator for the first time in your cluster, you can use the latest version or a specific version, run with the following commands:

```bash
kubectl create -f https://solr.apache.org/operator/downloads/crds/v0.5.1/all-with-dependencies.yaml
helm install solr-operator apache-solr/solr-operator --version 0.5.1
```

The command deploys the solr-operator on the Kubernetes cluster with the default configuration.
The [configuration](#chart-values) section lists the parameters that can be configured during installation.

_Note that the Helm chart version does not contain a `v` prefix, which the downloads version does. The Helm chart version is the only part of the Solr Operator release that does not use the `v` prefix._

### Upgrading the Solr Operator

If you are upgrading your Solr Operator deployment, you should always use a specific version of the chart and pre-install the Solr CRDS:

```bash
kubectl replace -f https://solr.apache.org/operator/downloads/crds/v0.5.1/all-with-dependencies.yaml
helm upgrade solr-operator apache-solr/solr-operator --version 0.5.1
```

#### Namespaces

If you want to specify the namespace for the installation, use the `--namespace` flag.
All resources will be deployed to the given namespace.

```bash
helm install solr-operator apache-solr/solr-operator --namespace solr
```

If you want to only watch that namespace, or others, then you will have to provide the `watchNamespaces` option.

```bash
# Watch the namespace where the operator is deployed to (just pass the boolean true)
helm install solr-operator apache-solr/solr-operator --namespace solr --set watchNamespaces=true
# Watch a single namespace different than the one being deployed to
helm install solr-operator apache-solr/solr-operator --namespace solr --set watchNamespaces=other
# Watch multiple namespaces (commmas must be escaped in the set string)
helm install solr-operator apache-solr/solr-operator --namespace solr --set watchNamespaces="team1\,team2\,team3"
```

Note: Passing `false` or `""` to the `watchNamespaces` variable will both result in the operator watchting all namespaces in the Kube cluster.

### Managing CRDs

Helm 3 automatically installs the Solr CRDs in the /crds directory, so no further action is needed when first installing the Operator.

If you have solr operator installations in multiple namespaces that are managed separately, you will likely want to skip installing CRDs when installing the chart.
This can be done with the `--skip-crds` helm option.

```bash
helm install solr-operator apache-solr/solr-operator --skip-crds --namespace solr
```

**Helm will not upgrade CRDs once they have been installed.
Therefore, if you are upgrading from a previous Solr Operator version, be sure to install the most recent CRDs first.**

You can update the released Solr CRDs with the following URL:
```bash
kubectl replace -f "https://solr.apache.org/operator/downloads/crds/<version>/<name>.yaml"
```

Examples:
- `https://solr.apache.org/operator/downloads/crds/v0.3.0/all.yaml`  
  Includes all Solr CRDs in the `v0.3.0` release
- `https://solr.apache.org/operator/downloads/crds/v0.2.7/all-with-dependencies.yaml`  
  Includes all Solr CRDs and dependency CRDs in the `v0.2.7` release
- `https://solr.apache.org/operator/downloads/crds/v0.2.8/solrclouds.yaml`  
  Just the SolrCloud CRD in the `v0.2.8` release

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

### Configuring the Solr Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| watchNamespaces | string | `""` | A comma-separated list of namespaces that the solr operator should watch. If empty, the solr operator will watch all namespaces in the cluster. If set to `true`, this will be populated with the namespace that the operator is deployed to. |
| zookeeper-operator.install | boolean | `true` | This option installs the Zookeeper Operator as a helm dependency |
| zookeeper-operator.use | boolean | `false` | This option enables the use of provided Zookeeper instances for SolrClouds via the Zookeeper Operator, without installing the Zookeeper Operator as a dependency. If `zookeeper-operator.install`=`true`, then this option is ignored. |
| leaderElection.enable | boolean | `true` | Enable leader election for the Solr Operator. Will work across multiple `watchNamespaces`, as long as all deployments have the same list for `watchNamespaces`. |
| metrics.enable | boolean | `true` | Enable metrics for the Solr Operator. Will be available via the "metrics"/8080 port on the solr operator pods under the "/metrics" path. |
| mTLS.clientCertSecret | string | `""` | Name of a Kubernetes TLS secret, in the same namespace, that contains a Client certificate to load into the operator. If provided, this is used when communicating with Solr. |
| mTLS.caCertSecretKey | string | `""` | Name of a Kubernetes secret, in the same namespace, that contains PEM encoded Root CA Certificate to use when connecting to Solr with Client Auth. |
| mTLS.caCertSecret | string | `""` | Name of the key in the `caCertSecret` that contains the Root CA Cert as a value. |
| mTLS.insecureSkipVerify | boolean | `true` | Skip server certificate and hostname verification when connecting to Solr with ClientAuth. |
| mTLS.watchForUpdates | boolean | `true` | Watch for updates to the mTLS certificate to reload the HTTP client used to call Solr pods with an updated client certificate. |

### Running the Solr Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| image.repository | string | `"apache/solr-operator"` | The repository of the Solr Operator image |
| image.tag | string | `"v0.5.1"` | The tag/version of the Solr Operator to run |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| fullnameOverride | string | `""` | A custom name for the Solr Operator Deployment |
| nameOverride | string | `""` |  |
| replicaCount | int | `1` | The number of Solr Operator pods to run
| rbac.create | boolean | `true` | Create the necessary RBAC rules, whether cluster-wide or namespaced, for the Solr Operator. |
| serviceAccount.create | boolean | `true` | Create a serviceAccount to be used for this operator. This serviceAccount will be given the permissions specified in the operator's RBAC rules. |
| serviceAccount.name | string | `""` | If `serviceAccount.create` is set to `false`, the name of an existing serviceAccount in the target namespace **must** be provided to run the Solr Operator with. This serviceAccount with be given the operator's RBAC rules. | |
| resources.limits | map[string]string |  | Provide Resource limits for the Solr Operator container |
| resources.requests | map[string]string |  | Provide Resource requests for the Solr Operator container |
| labels | map[string]string |  | Custom labels to add to the Solr Operator pod |
| annotations | map[string]string |  | Custom annotations to add to the Solr Operator pod |
| nodeSelector | map[string]string |  | Add a node selector for the Solr Operator pod, to specify where it can be scheduled |
| affinity | object |  | Add Kubernetes affinity information for the Solr Operator pod |
| tolerations | []object |  | Specify a list of Kubernetes tolerations for the Solr Operator pod |
| priorityClassName | string | `""` | Give a priorityClassName for the Solr Operator pod |
| sidecarContainers | []object |  | An optional list of additional containers to run along side the Solr Operator in its pod |

### Configuring the Zookeeper Operator

If you have `zookeeper-operator.install` set to `true`, which is the default behavior, then you can use the [Zookeeper Operator Chart values](https://github.com/pravega/zookeeper-operator/tree/v0.2.12/charts/zookeeper-operator#configuration).
You must prefix every Zookeeper Operator configuration with `zookeeper-operator.`, in order for it to be used.

For example:
```yaml
zookeeper-operator:
  rbac:
    create: false
  watchNamespace: test-namespace
```
