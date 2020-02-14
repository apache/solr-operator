solr-operator
=============
A Helm chart for [Solr Operator]

## Installing the Chart

To install the chart with the release name `my-release`:

```console
$ helm install --name my-release
```

The command deploys envoy on the Kubernetes cluster with the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.

## About CRDs

Helm 2:

If you are using Helm 2, CRDs are installed using the crd-install hook. Prior to installing, you'll need to uncomment the last two lines in [kustomization.yaml](../../config/crd/kustomization.yaml), and run `make manifests`

Helm 3:

Helm 3 automatically runs CRDs in the /crds directory, no further action is needed.

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```console
$ helm delete my-release
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
