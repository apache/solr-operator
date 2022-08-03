<!--
    Licensed to the Apache Software Foundation (ASF) under one or more
    contributor license agreements.  See the NOTICE file distributed with
    this work for additional information regarding copyright ownership.
    The ASF licenses this file to You under the Apache License, Version 2.0
    the "License"); you may not use this file except in compliance with
    the License.  You may obtain a copy of the License at

        http://www.apache.org/licenses/LICENSE-2.0

    Unless required by applicable law or agreed to in writing, software
    distributed under the License is distributed on an "AS IS" BASIS,
    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    See the License for the specific language governing permissions and
    limitations under the License.
 -->

# Solr Operator Upgrade Notes

Please carefully read the entries for all versions between the version you are running and the version you want to upgrade to.

Ensure to read the [Upgrade Warnings and Notes](#upgrade-warnings-and-notes) for the version you are upgrading to as well as the versions you are skipping.

If you want to skip versions when upgrading, be sure to check out the [upgrading minor versions](#upgrading-minor-versions-v_x_) and [upgrading patch versions](#upgrading-patch-versions-v__x) sections.

## Version Compatibility Matrixes

### Kubernetes Versions

| Solr Operator Version | `1.15` | `1.16` - `1.18` |  `1.19` - `1.21` | `1.22`+ |
|:---------------------:| :---: | :---: | :---: | :---: |
|       `v0.2.6`        | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :x: |
|       `v0.2.7`        | :x: | :heavy_check_mark: | :heavy_check_mark: | :x: |
|       `v0.2.8`        | :x: | :heavy_check_mark: | :heavy_check_mark: | :x: |
|       `v0.3.x`        | :x: | :heavy_check_mark: | :heavy_check_mark: | :x: |
|       `v0.4.x`        | :x: | :heavy_check_mark: | :heavy_check_mark: | :x: |
|       `v0.5.x`        | :x: | :x: | :heavy_check_mark: | :heavy_check_mark: |
|       `v0.6.x`        | :x: | :x: | :heavy_check_mark: | :heavy_check_mark: |

### Solr Versions

| Solr Operator Version | `6.6` | `7.7` | `8.0` - `8.5` | `8.6`+ |
|:---------------------:| :---: | :---: | :---: | :---: |
|       `v0.2.6`        | :grey_question: | :heavy_check_mark: | :heavy_check_mark: | :x: |
|       `v0.2.7`        | :grey_question: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
|       `v0.2.8`        | :grey_question: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
|       `v0.3.x`        | :grey_question: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
|       `v0.4.x`        | :grey_question: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
|       `v0.5.x`        | :grey_question: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
|       `v0.6.x`        | :grey_question: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |

Please note that this represents basic compatibility with the Solr Operator.
There may be options and features that require newer versions of Solr.
(e.g. S3/GCS Backup Support)

Please test to make sure the features you plan to use are compatible with the version of Solr you choose to run.


### Upgrading from `v0.2.x` to `v0.3.x`
If you are upgrading from `v0.2.x` to `v0.3.x`, please follow the [Upgrading to Apache guide](upgrading-to-apache.md).
This is a special upgrade that requires different instructions.

### Upgrading minor versions (`v_.X._`)

In order to upgrade minor versions (e.g. `v0.2.5` -> `v0.3.0`), you must upgrade one minor version at a time (e.g. `v0.2.0` -> `v0.3.0` -> `v0.4.0`).
It is also necessary to upgrade to the latest patch version before upgrading to the next minor version.
Therefore if you are running `v0.2.5` and you want to upgrade to `v0.3.0`, you must first upgrade to `v0.2.8` before upgrading to `v0.3.0`.

### Upgrading patch versions (`v_._.X`)

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
kubectl replace -f "http://solr.apache.org/operator/downloads/crds/v0.7.0-prerelease/all-with-dependencies.yaml"
helm upgrade solr-operator apache-solr/solr-operator --version 0.7.0-prerelease
```

_Note that the Helm chart version does not contain a `v` prefix, which the downloads version does. The Helm chart version is the only part of the Solr Operator release that does not use the `v` prefix._

## Upgrade Warnings and Notes

### v0.6.0
- The default Solr version for the `SolrCloud` and `SolrPrometheusExporter` resources has been upgraded from `8.9` to `8.11`.
  This will not affect any existing resources, as default versions are hard-written to the resources immediately.
  Only new resources created after the Solr Operator is upgraded to `v0.6.0` will be affected.

- The required version of the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator) to use with this version has been upgraded from `v0.2.12` to `v0.2.14`.
  If you use the Solr Operator helm chart, then by default the new version of the Zookeeper Operator will be installed as well.
  Refer to the helm chart documentation if you want to manage the Zookeeper Operator installation yourself.  
  Please refer to the [Zookeeper Operator release notes](https://github.com/pravega/zookeeper-operator/releases) before upgrading.
  Make sure to install the correct version of the Zookeeper Operator CRDS, as [shown above](#upgrading-the-zookeeper-operator).

- The SolrCloud CRD field `Spec.solrAddressability.external.additionalDomains` has been renamed to `additionalDomainNames`.
  In this release `additionalDomains` is still accepted, but all values will automatically be added to `additionalDomainNames` and the field will be set to `nil` by the operator.
  The `additionalDomains` option will be removed in a future version.

- The SolrCloud CRD field `Spec.solrAddressability.external.ingressTLSTerminationSecret` has been moved to `Spec.solrAddressability.external.ingressTLSTermination.tlsSecret`.
  In this release `ingressTLSTerminationSecret` is still accepted, but all values will automatically be changed to `ingressTLSTermination.tlsSecret` and the original field will be set to `nil` by the operator.
  The `ingressTLSTerminationSecret` option will be removed in a future version.

- `SolrPrometheusExporter` resources without any image specifications (`SolrPrometheusExporter.Spec.image.*`) will use the referenced `SolrCloud` image, if the reference is by `name`, not `zkConnectionString`.
  If any `SolrPrometheusExporter.Spec.image.*` option is provided, then those values will be defaulted by the Solr Operator and the `SolrCloud` image will not be used.
  When upgrading from `v0.5.*` to `v0.6.0`, only new `SolrPrometheusExporter` resources will use this new feature.
  To enable it on existing resources, update the resources and remove the `SolrPrometheusExporter.Spec.image` section.

- CRD options deprecated in `v0.5.0` have been removed.
  This includes field `SolrCloud.spec.dataStorage.backupRestoreOptions`, `SolrBackup.spec.persistence` and `SolrBackup.status.persistenceStatus`.
  Upgrading to `v0.5.*` will remove these options on existing and new SolrCloud and SolrBackup resources.
  However, once the Solr CRDs are upgraded to `v0.6.0`, you will no longer be able to submit resources with the options listed above.
  Please migrate your systems to use the new options while running `v0.5.*`, before upgrading to `v0.6.0`. 

### v0.5.0
- Due to the deprecation and removal of `networking.k8s.io/v1beta1` in Kubernetes v1.22, `networking.k8s.io/v1` will be used for Ingresses.

  **This means that Kubernetes support is now limited to 1.19+.**  
  If you are unable to use a newer version of Kubernetes, please install the `v0.4.0` version of the Solr Operator for use with Kubernetes 1.18 and below.
  See the [version compatibility matrix](#kubernetes-versions) for more information.

  This also means that if you specify a custom `ingressClass` via an annotation, you should change to use the `SolrCloud.spec.customSolrKubeOptions.ingressOptions.ingressClassName` instead.
  The ability to set the class through annotations is now deprecated in Kubernetes and will be removed in future versions.

- The legacy way of specifying a backupRepository has been **DEPRECATED**.
  Instead of using `SolrCloud.spec.dataStorage.backupRestoreOptions`, use `SolrCloud.spec.backupRepositories`.
  The `SolrCloud.spec.dataStorage.backupRestoreOptions` option **will be removed in `v0.6.0`**.  
  **Note**: Do not take backups while upgrading from the Solr Operator `v0.4.0` to `v0.5.0`.
  Wait for the SolrClouds to be updated, after the Solr Operator is upgraded, and complete their rolling restarts before continuing to use the Backup functionality.

- The location of Solr backup data as well as the name of the Solr backups have been changed, when using volume repositories.
  Previously the name of the backup (in solr) was set to the name of the collection.
  Now the name given to the backup in Solr will be set to `<backup-resource-name>-<collection-name>`, without the `<` or `>` characters, where the `backup-resource-name` is the name of the SolrBackup resource.

  The directory in the Read-Write-Many Volume, required for volume repositories, that backups are written to is now `/cloud/<solr-cloud-name>/backups` by default, instead of `/cloud/<solr-cloud-name>/backups/<backup-name>`.
  Because the backup name in Solr uses both the SolrBackup resource name and the collection name, there should be no collisions in this directory.
  However, this can be overridden using the `SolrBackup.spec.location` option, which is appended to `/cloud/<solr-cloud-name>`.

- The SolrBackup persistence option has been removed as of `v0.5.0`.
  Users should plan to keep their backup data in the shared volume if using a Volume Backup repository.
  If `SolrBackup.spec.persistence` is provided, it will be removed and written back to Kubernetes.

  Users using the S3 persistence option should try to use the [S3 backup repository](solr-backup/README.md#s3-backup-repositories) instead. This requires Solr 8.10 or higher.

- Default ports when using TLS are now set to 443 instead of 80.
  This affects `solrCloud.Spec.SolrAddressability.CommonServicePort` and `solrCloud.Spec.SolrAddressability.CommonServicePort` field defaulting.
  Users already explicitly setting these values will not be affected.

### v0.4.0
- The required version of the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator) to use with this version has been upgraded from `v0.2.9` to `v0.2.12`.
  If you use the Solr Operator helm chart, then by default the new version of the Zookeeper Operator will be installed as well.
  Refer to the helm chart documentation if you want to manage the Zookeeper Operator installation yourself.  
  Please refer to the [Zookeeper Operator release notes](https://github.com/pravega/zookeeper-operator/releases) before upgrading.
  Make sure to install the correct version of the Zookeeper Operator CRDS, as [shown above](#upgrading-the-zookeeper-operator).

- The deprecated Solr Operator Helm chart option `useZkOperator` has been removed, use `zookeeper-operator.use` instead.  
  **Note**: The old option takes a _string_ `"true"`/`"false"`, while the new option takes a _boolean_ `true`/`false`.

- The default Solr version for `SolrCloud` and `SolrPrometheusExporter` resources has been upgraded from `7.7.0` to `8.9`.
  This will not affect any existing resources, as default versions are hard-written to the resources immediately.
  Only new resources created after the Solr Operator is upgraded to `v0.4.0` will be affected.

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
- Due to the addition of possible sidecar/initContainers for SolrClouds, the version of CRDs used had to be upgraded to `apiextensions.k8s.io/v1`.

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
