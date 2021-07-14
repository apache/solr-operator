# Solr Operator Upgrade Notes

Please carefully read the entries for all versions between the version you are running and the version you want to upgrade to.

## Upgrading from `v0.2.x` to `v0.3.x`
If you are upgrading from `v0.2.x` to `v0.3.x`, please follow the [Upgrading to Apache guide](upgrading-to-apache.md).
This is a special upgrade that requires different instructions.

## Upgrading minor versions (`v_.X._`)

In order to upgrade minor versions (e.g. `v0.2.5` -> `v0.3.0`), you must upgrade one minor version at a time (e.g. `v0.2.0` -> `v0.3.0` -> `v0.4.0`).
It is also necessary to upgrade to the latest patch version before upgrading to the next minor version.
Therefore if you are running `v0.2.5` and you want to upgrade to `v0.3.0`, you must first upgrade to `v0.2.8` before upgrading to `v0.3.0`.

## Upgrading patch versions (`v_._.X`)

You should be able to upgrade from a version to any patch version with the same minor and major versions.
It is always encouraged to upgrade to the latest patch version of the minor and major version you are running.
There is no need to upgrade one patch version at a time (e.g. `v0.2.5` -> `v0.2.6` -> `v0.2.7` -> `v0.2.8`),
instead you can leap to the latest patch version (e.g. `v0.2.5` -> `v0.2.8`).

## Installing the Solr Operator vs Solr CRDs

Installing the Solr Operator, especially via the [Helm Chart](https://artifacthub.io/packages/helm/apache-solr/solr-operator),
does not necessarily mean that you are installing the required CRDs for that version of the Solr Operator.

If the Solr CRDs already exist in your Kubernetes cluster, then Helm will not update them even if the CRDs have changed between the Helm chart versions.
Instead, you will need to manually install the CRDs whenever upgrading your Solr Operator.  
**You should always upgrade your CRDs before upgrading the Operator**

You can do this via the following command, replacing `<version>` with the version of the Solr Operator you are installing:
```bash
# Just replace the Solr CRDs
kubectl replace -f "http://solr.apache.org/operator/downloads/crds/<version>/all.yaml"
# Just replace the Solr CRDs and all CRDs it might depend on (e.g. ZookeeperCluster)
kubectl replace -f "http://solr.apache.org/operator/downloads/crds/<version>/all-with-dependencies.yaml"
```

It is **strongly recommended** to use `kubectl create` or `kubectl replace`, instead of `kubectl apply` when creating/updating CRDs.

### Upgrading the Zookeeper Operator

When upgrading the Solr Operator, you may need to upgrade the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator) at the same time.
If you are using the Solr Helm chart to deploy the Zookeeper operator, then you won't need to do anything besides installing the CRD's with dependencies, and upgrade the Solr Operator helm deployment.

```bash
# Just replace the Solr CRDs and all CRDs it might depend on (e.g. ZookeeperCluster)
kubectl replace -f "http://solr.apache.org/operator/downloads/crds/v0.3.0/all-with-dependencies.yaml"
helm upgrade solr-operator apache-solr/solr-operator --version 0.3.0
```

_Note that the Helm chart version does not contain a `v` prefix, which the downloads version does. The Helm chart version is the only part of the Solr Operator release that does not use the `v` prefix._

## Upgrade Warnings and Notes

### v0.4.0
- In previous versions of the Solr Operator, the provided Zookeeper instances could only use Persistent Storage.
  Now ephemeral storage is enabled, and used by default if Solr is using ephemeral storage.
  The ZK storage type can be explicitly set via `Spec.zookeeperRef.provided.ephemeral` or `Spec.zookeeperRef.provided.persistence`,
  however if neither is set, the Solr Operator will default to use the type of storage (persistent or ephemeral) that Solr is using.  
  **This means that the default Zookeeper Storage type can change for users using ephemeral storage for Solr.
  If you require ephemeral Solr storage and persistent Zookeeper Storage, be sure to explicitly set that starting in `v0.4.0`.**

### v0.3.0
- All deprecated CRD fields and Solr Operator options from `v0.2.*` have been removed.

- The `SolrCollection` and `SolrCollectionAlias` have been removed. Please use the Solr APIs to manage these resources instead.
  Discussion around the removal can be found in [Issue #204](https://github.com/apache/solr-operator/issues/204).

- The required version of the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator) to use with this version has been upgraded from `v0.2.6` to `v0.2.9`.
  If you use the Solr Operator helm chart, then by default the new version of the Zookeeper Operator will be installed as well.
  Refer to the helm chart documentation if you want to manage the Zookeeper Operator installation yourself.  
  Please refer to the [Zookeeper Operator release notes](https://github.com/pravega/zookeeper-operator/releases) before upgrading.

### v0.2.7
- Do to the addition of possible sidecar/initContainers for SolrClouds, the version of CRDs used had to be upgraded to `apiextensions.k8s.io/v1`.

  **This means that Kubernetes support is now limited to 1.16+.**  
  If you are unable to use a newer version of Kubernetes, please install the `v0.2.6` version of the Solr Operator for use with Kubernetes 1.15 and below.

- The location of backup-restore volume mounts in Solr containers has changed from `/var/solr/solr-backup-restore` to `/var/solr/data/backup-restore`.
  This change was made to ensure that there were no issues using the backup API with solr 8.6+, which restricts the locations that backup data can be saved to and read from.
  This change should be transparent if you are merely using the SolrBackup CRD.
  All files permissions issues with SolrBackups should now be addressed.

- The default `PodManagementPolicy` for StatefulSets has been changed to `Parallel` from `OrderedReady`.
  This change will not affect existing StatefulSets, as `PodManagementPolicy` cannot be updated.
  In order to continue using `OrderedReady` on new SolrClouds, please use the following setting:  
  `SolrCloud.spec.customSolrKubeOptions.statefulSetOptions.podManagementPolicy`

- The `SolrCloud` and `SolrPrometheusExporter` services' portNames have changed to `"solr-client"` and `"solr-metrics"` from `"ext-solr-client"` and `"ext-solr-metrics"`, respectively.
  This is due to a bug in Kubernetes where `portName` and `targetPort` must match for services.

- Support for `etcd`/`zetcd` deployments has been removed.  
  The section for a Zookeeper cluster Spec `SolrCloud.spec.zookeeperRef.provided.zookeeper` has been **DEPRECATED**.
  The same fields (except for the deprecated `persistentVolumeClaimSpec` option) are now available under `SolrCloud.spec.zookeeperRef.provided`.

- Data Storage options have been expanded, and moved from their old locations.
    - `SolrCloud.spec.dataPvcSpec` has been **DEPRECATED**.  
      Please instead use the following instead: `SolrCloud.spec.dataStorage.persistent.pvcTemplate.spec=<spec>`
    - `SolrCloud.spec.backupRestoreVolume` has been **DEPRECATED**.  
      Please instead use the following instead: `SolrCloud.spec.dataStorage.backupRestoreOptions.Volume=<volume-source>`

### v0.2.6
- The solr-operator argument `--ingressBaseDomain` has been **DEPRECATED**.
  In order to set the external baseDomain of your clouds, please begin to use `SolrCloud.spec.solrAddressability.external.domainName` instead.
  You will also need to set `SolrCloud.spec.solrAddressability.external.method` to `Ingress`.
  The `--ingressBaseDomain` argument is backwards compatible, and all existing SolrCloud objects will be auto-updated once your operator is upgraded to `v0.2.6`.
  The argument will be removed in a future version (`v0.3.0`).

### v0.2.4
- The default supported version of the Zookeeper Operator has been upgraded to `v0.2.6`.  
  If you are using the provided zookeeper option for your SolrClouds, then you will want to upgrade your zookeeper operator version as well as the version and image of the zookeeper that you are running.
  You can find examples of the zookeeper operator as well as solrClouds that use provided zookeepers in the [examples](/example) directory.  
  Please refer to the [Zookeeper Operator release notes](https://github.com/pravega/zookeeper-operator/releases) before upgrading.

### v0.2.3
- If you do not use an ingress with the Solr Operator, the Solr Hostname and Port will change when upgrading to this version. This is to fix an outstanding bug. Because of the headless service port change, you will likely see an outage for inter-node communication until all pods have been restarted.

### v0.2.2
- `SolrCloud.spec.solrPodPolicy` has been **DEPRECATED** in favor of the `SolrCloud.spec.customSolrKubeOptions.podOptions` option.  
  This option is backwards compatible, but will be removed in a future version (`v0.3.0`).

- `SolrPrometheusExporter.spec.solrPodPolicy` has been **DEPRECATED** in favor of the `SolrPrometheusExporter.spec.customKubeOptions.podOptions` option.  
  This option is backwards compatible, but will be removed in a future version (`v0.3.0`).

### v0.2.1
- The zkConnectionString used for provided zookeepers changed from using the string provided in the `ZkCluster.Status`, which used an IP, to using the service name. This will cause a rolling restart of your solrs using the provided zookeeper option, but there will be no data loss.

### v0.2.0
- Uses `gomod` instead of `dep`
- `SolrCloud.spec.zookeeperRef.provided.zookeeper.persistentVolumeClaimSpec` has been **DEPRECATED** in favor of the `SolrCloud.zookeeperRef.provided.zookeeper.persistence` option.  
  This option is backwards compatible, but will be removed in a future version (`v0.3.0`).
- An upgrade to the ZKOperator version `0.2.4` is required.