Solr Operator
=============

The Solr Operator is designed to allow easy deployment Solr Clouds and other Solr Resources to Kubernetes.

Documentation around using the Solr Operator can be found in it's [source repo](https://github.com/apache/lucene-solr-operator).

## Using the Helm Chart

### Installing the Zookeeper Operator

Before installing the Solr Operator, we need to install the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator).
This is because the Solr Operator, in most instances, relies on the Zookeeper Operator to create the Zookeeper clusters that Solr coordinates through.

The Solr Operator helm chart has a conditional dependency on the [official Zookeeper Operator helm chart](https://github.com/pravega/zookeeper-operator/tree/master/charts/zookeeper-operator#installing-the-chart),
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
helm repo add apache-solr https://apache.github.io/lucene-solr-operator/charts
```

### Installing the Chart

To install the Solr Operator with the latest version or with a specific version, run either of the following commands:

```bash
helm install solr-operator apache-solr/solr-operator
helm install solr-operator apache-solr/solr-operator --version 0.3.0
```

The command deploys the solr-operator on the Kubernetes cluster with the default configuration.
The [configuration](#chart-values) section lists the parameters that can be configured during installation.

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

Helm 3 automatically runs CRDs in the /crds directory, no further action is needed.

If have solr operator installations in multiple namespaces that are managed separately, you will likely want to skip installing CRDs when installing the chart.
This can be done with the `--skip-crds` helm option.

```bash
helm install solr-operator apache-solr/solr-operator --skip-crds --namespace solr
```

Helm will not upgrade CRDs once they have been installed.
Therefore, if you are upgrading from a previous Solr Operator version, be sure to install the most recent CRDs **first**.

### Uninstalling the Chart

To uninstall/delete the Solr Operator, run:

```bash
helm uninstall solr-operator
```

The command removes all the Kubernetes components associated with the chart and deletes the release, except for the Solr CRDs.

**NOTE: If you are using the Zookeeper Operator helm chart as a dependency, this will also delete the ZookeeperCluster CRD, thus deleting all Zookeeper instances in the Kubernetes Cluster.**


## Chart Values

### Configuring the Solr Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| watchNamespaces | string | `""` | A comma-separated list of namespaces that the solr operator should watch. If empty, the solr operator will watch all namespaces in the cluster. If set to `true`, this will be populated with the namespace that the operator is deployed to. |
| zookeeper-operator.install | boolean | `true` | This option installs the Zookeeper Operator as a helm dependency |
| zookeeper-operator.use | boolean | `false` | This option enables the use of provided Zookeeper instances for SolrClouds via the Zookeeper Operator, without installing the Zookeeper Operator as a dependency. If `zookeeper-operator.install`=`true`, then this option is ignored. |
| useZkOperator | string | `"true"` | **DEPRECATED** Replaced by the _boolean_ "zookeeper-operator.use" option. This option will be removed in v0.4.0 |

### Running the Solr Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| image.repository | string | `"apache/solr-operator"` | The repository of the Solr Operator image |
| image.tag | string | `"v0.2.8"` | The tag/version of the Solr Operator to run |
| image.pullPolicy | string | `"Always"` |  |
| fullnameOverride | string | `""` | A custom name for the Solr Operator Deployment |
| nameOverride | string | `""` |  |
| replicaCount | int | `1` | The number of Solr Operator pods to run |
| resources.limits.cpu | string | `"400m"` |  |
| resources.limits.memory | string | `"500Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"100Mi"` |  |
| rbac.create | boolean | true | Create the necessary RBAC rules, whether cluster-wide or namespaced, for the Solr Operator. |
| serviceAccount.create | boolean | true | Create a serviceAccount to be used for this operator. This serviceAccount will be given the permissions specified in the operator's RBAC rules. |
| serviceAccount.name | string | "" | If `serviceAccount.create` is set to `false`, the name of an existing serviceAccount in the target namespace **must** be provided to run the Solr Operator with. This serviceAccount with be given the operator's RBAC rules. | 
