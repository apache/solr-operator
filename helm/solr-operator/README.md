Solr Operator
=============
A Helm chart for the Solr Operator.

## The Solr Operator

The Solr Operator is designed to allow easy deployment Solr Clouds and other Solr Resources to Kubernetes.

Documentation around using the Solr Operator can be found in it's [source repo](https://github.com/bloomberg/solr-operator).

## Using the Helm Chart

### Installing the Chart

To install the chart with the release name `test`:

```console
$ helm install test .
```

The command deploys the solr-operator on the Kubernetes cluster with the default configuration. The [configuration](#chart-values) section lists the parameters that can be configured during installation.

**NOTE**: Since by default the `useZkOperator` option is set to `True`, you must have already installed the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator) in your kubernetes cluster. A helm chart is [also available](https://github.com/pravega/zookeeper-operator/blob/master/charts/zookeeper-operator/Chart.yaml) for it.

### Helm Version Differences

#### Helm 2

If you are using Helm 2, CRDs are installed using the crd-install hook. Prior to installing, you'll need to uncomment the last two lines in [kustomization.yaml](../../config/crd/kustomization.yaml), and run `make manifests`

You will also need to update the install command to use the name flag, as shown below.

```console
$ helm install --name test .
```

#### Helm 3

Helm 3 automatically runs CRDs in the /crds directory, no further action is needed.

### Uninstalling the Chart

To uninstall/delete the `solr-operator` deployment:

#### Helm 3

```console
$ helm uninstall test
```

#### Helm 2

```console
$ helm delete test
```

The command removes all the Kubernetes components associated with the chart and deletes the release.


## Chart Values

### Running the Solr Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| image.repository | string | `"bloomberg/solr-operator"` | The repository of the Solr Operator image |
| image.tag | string | `"v0.2.1"` | The tag/version of the Solr Operator to run |
| image.pullPolicy | string | `"Always"` |  |
| fullnameOverride | string | `""` | A custom name for the Solr Operator Deployment |
| nameOverride | string | `""` |  |
| replicaCount | int | `1` | The number of Solr Operator pods to run |
| resources.limits.cpu | string | `"400m"` |  |
| resources.limits.memory | string | `"500Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"100Mi"` |  |

### Configuring the Solr Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| ingressBaseDomain | string | `""` | If you have a base domain that points to your ingress controllers for this kubernetes cluster, you can provide this. SolrClouds will then begin to use ingresses that utilize this base domain. E.g. `solrcloud-test.<base.domain>` |
| useZkOperator | string | `"true"` | This option enables the use of provided Zookeeper instances for SolrClouds |
| useEtcdOperator | string | `"false"` | This option enables the use of provided Zetcd instances for SolrClouds |
