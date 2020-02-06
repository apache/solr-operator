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

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| useEtcdOperator | string | `"true"` |  |
| fullnameOverride | string | `""` |  |
| image.pullPolicy | string | `"Always"` |  |
| image.repository | string | `"bloomberg/solr-operator"` |  |
| image.tag | string | `"v0.2.1"` |  |
| ingressBaseDomain | string | `""` |  |
| nameOverride | string | `""` |  |
| replicaCount | int | `1` |  |
| resources.limits.cpu | string | `"400m"` |  |
| resources.limits.memory | string | `"500Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"100Mi"` |  |
| useZkOperator | string | `"true"` |  |