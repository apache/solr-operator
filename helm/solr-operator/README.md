Solr Operator
=============

The Solr Operator is designed to allow easy deployment Solr Clouds and other Solr Resources to Kubernetes.

Documentation around using the Solr Operator can be found in it's [source repo](https://github.com/apache/lucene-solr-operator).

## Using the Helm Chart

### Installing the Zookeeper Operator

Before installing the Solr Operator, we need to install the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator).
This is because the Solr Operator, in most instances, relies on the Zookeeper Operator to create the Zookeeper clusters that Solr coordinates through.

If you are setting `useZkOperator=false`, then please disregard this section.

Eventually the Zookeeper Opertor will be a dependency on the Solr Operator helm chart, but for now there are two easy ways to deploy it.

- A helm chart is available [in the Zookeeper Operator repository](https://github.com/pravega/zookeeper-operator/blob/master/charts/zookeeper-operator/).
- Use the following kubectl command:
```bash
kubectl apply -f https://raw.githubusercontent.com/apache/lucene-solr-operator/main/example/dependencies/zk_operator.yaml
```

#### Helm

Eventually this will be a dependency on the helm chart, but for now we can run an easy `kubectl apply`.

```bash
kubectl apply -f https://raw.githubusercontent.com/apache/lucene-solr-operator/main/example/dependencies/zk_operator.yaml
```

### Adding the Solr Operator Helm Chart Repository
You should only need to add the solr operator helm chart repository once, by running the following command:

```console
$ helm repo add solr-operator https://apache.github.io/lucene-solr-operator/charts
```

### Installing the Chart

To install the Solr Operator with the latest version or with a specific version, run either of the following commands:

```console
$ helm install solr-operator solr-operator/solr-operator
$ helm install solr-operator solr-operator/solr-operator --version 0.2.5
```

The command deploys the solr-operator on the Kubernetes cluster with the default configuration.
The [configuration](#chart-values) section lists the parameters that can be configured during installation.

#### Namespaces

If you want to specify the namespace for the installation, use the `--namespace` flag.
All resources will be deployed to the given namespace.

```console
$ helm install solr-operator solr-operator/solr-operator --namespace solr
```

If you want to only watch that namespace, or others, then you will have to provide the `watchNamespaces` option.

```console
// Watch the namespace where the operator is deployed to (just pass the boolean true)
$ helm install solr-operator solr-operator/solr-operator --namespace solr --set watchNamespaces=true
// Watch a single namespace different than the one being deployed to
$ helm install solr-operator solr-operator/solr-operator --namespace solr --set watchNamespaces=other
// Watch multiple namespaces (commmas must be escaped in the set string)
$ helm install solr-operator solr-operator/solr-operator --namespace solr --set watchNamespaces="team1\,team2\,team3"
```

Note: Passing `false` and `""` to the `watchNamespaces` variable will both result in the operator watchting all namespaces in the Kube cluster.

### Managing CRDs

#### Helm 3

Helm 3 automatically runs CRDs in the /crds directory, no further action is needed.

If have solr operator installations in multiple namespaces that are managed separately, you will likely want to skip installing CRDs when installing the chart.
This can be done with the `--skip-crds` helm option.

```console
$ helm install solr-operator solr-operator/solr-operator --skip-crds --namespace solr
```

#### Helm 2

If you are using Helm 2, CRDs are installed using the crd-install hook. Prior to installing, you'll need to uncomment the last two lines in [kustomization.yaml](../../config/crd/kustomization.yaml), and run `make manifests`

You will also need to update the install command to use the name flag, as shown below.

```console
$ helm install --name solr-operator solr-operator/solr-operator
```

### Uninstalling the Chart

To uninstall/delete the `solr-operator` deployment:

#### Helm 3

```console
$ helm uninstall solr-operator
```

#### Helm 2

```console
$ helm delete solr-operator
```

The command removes all the Kubernetes components associated with the chart and deletes the release.


## Chart Values

### Configuring the Solr Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| watchNamespaces | string | `""` | A comma-separated list of namespaces that the solr operator should watch. If empty, the solr operator will watch all namespaces in the cluster. If set to `true`, this will be populated with the namespace that the operator is deployed to. |
| useZkOperator | string | `"true"` | This option enables the use of provided Zookeeper instances for SolrClouds |
| ingressBaseDomain | string | `""` | **NOTE: This feature is deprecated and will be removed in `v0.3.0`. The option is now provided within the SolrCloud CRD.** If you have a base domain that points to your ingress controllers for this kubernetes cluster, you can provide this. SolrClouds will then begin to use ingresses that utilize this base domain. E.g. `solrcloud-test.<base.domain>` |

### Running the Solr Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| image.repository | string | `"bloomberg/solr-operator"` | The repository of the Solr Operator image |
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
