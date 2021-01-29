# The SolrCloud CRD

The SolrCloud CRD allows users to spin up a Solr cloud in a very configurable way.
Those configuration options are laid out on this page.

## Data Storage

The SolrCloud CRD gives the option for users to use either
persistent storage, through [PVCs](https://kubernetes.io/docs/concepts/storage/persistent-volumes/),
or ephemeral storage, through [emptyDir volumes](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir),
to store Solr data.
Ephemeral and persistent storage cannot be used together, if both are provided, the `persistent` options take precedence.
If neither is provided, ephemeral storage will be used by default.

These options can be found in `SolrCloud.spec.dataStorage`

- **`persistent`**
  - **`reclaimPolicy`** - Either `Retain`, the default, or `Delete`.
    This describes the lifecycle of PVCs that are deleted after the SolrCloud is deleted, or the SolrCloud is scaled down and the pods that the PVCs map to no longer exist.
    `Retain` is used by default, as that is the default Kubernetes policy, to leave PVCs in case pods, or StatefulSets are deleted accidentally.
    
    Note: If reclaimPolicy is set to `Delete`, PVCs will not be deleted if pods are merely deleted. They will only be deleted once the `SolrCloud.spec.replicas` is scaled down or deleted.
  - **`pvcTemplate`** - The template of the PVC to use for the solr data PVCs. By default the name will be "data".
    Only the `pvcTemplate.spec` field is required, metadata is optional.
    
    Note: This template cannot be changed unless the SolrCloud is deleted and recreated.
    This is a [limitation of StatefulSets and PVCs in Kubernetes](https://github.com/kubernetes/enhancements/issues/661).
- **`ephemeral`**
  - **`emptyDir`** - An [`emptyDir` volume source](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir) that describes the desired emptyDir volume to use in each SolrCloud pod to store data.
  This option is optional, and if not provided an empty `emptyDir` volume source will be used.
    
- **`backupRestoreOptions`** (Required for integration with [`SolrBackups`](../solr-backup/README.md))
  - **`volume`** - This is a [volume source](https://kubernetes.io/docs/concepts/storage/volumes/), that supports `ReadWriteMany` access.
  This is critical because this single volume will be loaded into all pods at the same path.
  - **`directory`** - A custom directory to store backup/restore data, within the volume described above.
  This is optional, and defaults to the name of the SolrCloud.
  Only use this option when you require restoring the same backup to multiple SolrClouds.

## Update Strategy

The SolrCloud CRD provides users the ability to define how it is updated, through `SolrCloud.Spec.updateStrategy`.
This provides the following options:

Under `SolrCloud.Spec.updateStrategy`:

- **`method`** - The method in which Solr pods should be updated. Enum options are as follows:
  - `Managed` - (Default) The Solr Operator will take control over deleting pods for updates. This process is [documented here](managed-updates.md).
  - `StatefulSet` - Use the default StatefulSet rolling update logic, one pod at a time waiting for all pods to be "ready".
  - `Manual` - Neither the StatefulSet or the Solr Operator will delete pods in need of an update. The user will take responsibility over this.
- **`managed`** - Options for rolling updates managed by the Solr Operator.
  - **`maxPodsUnavailable`** - (Defaults to `"25%"`) The number of Solr pods in a Solr Cloud that are allowed to be unavailable during the rolling restart.
  More pods may become unavailable during the restart, however the Solr Operator will not kill pods if the limit has already been reached.  
  - **`maxShardReplicasUnavailable`** - (Defaults to `1`) The number of replicas for each shard allowed to be unavailable during the restart.
  
**Note:** Both `maxPodsUnavailable` and `maxShardReplicasUnavailable` are intOrString fields. So either an int or string can be provided for the field.
- **int** - The parameter is treated as an absolute value, unless the value is <= 0 which is interpreted as unlimited.
- **string** - Only percentage string values (`"0%"` - `"100%"`) are accepted, all other values will be ignored.
  - **`maxPodsUnavailable`** - The `maximumPodsUnavailable` is calculated as the percentage of the total pods configured for that Solr Cloud.
  - **`maxShardReplicasUnavailable`** - The `maxShardReplicasUnavailable` is calculated independently for each shard, as the percentage of the number of replicas for that shard.

## Addressability

The SolrCloud CRD provides users the ability to define how it is addressed, through the following options:

Under `SolrCloud.Spec.solrAddressability`:

- **`podPort`** - The port on which the pod is listening. This is also that the port that the Solr Jetty service will listen on. (Defaults to `8983`)
- **`commonServicePort`** - The port on which the common service is exposed. (Defaults to `80`)
- **`kubeDomain`** - Specifies an override of the default Kubernetes cluster domain name, `cluster.local`. This option should only be used if the Kubernetes cluster has been setup with a custom domain name.
- **`external`** - Expose the cloud externally, outside of the kubernetes cluster in which it is running.
  - **`method`** - (Required) The method by which your cloud will be exposed externally.
  Currently available options are [`Ingress`](https://kubernetes.io/docs/concepts/services-networking/ingress/) and [`ExternalDNS`](https://github.com/kubernetes-sigs/external-dns).
  The goal is to support more methods in the future, such as LoadBalanced Services.
  - **`domainName`** - (Required) The primary domain name to open your cloud endpoints on. If `useExternalAddress` is set to `true`, then this is the domain that will be used in Solr Node names.
  - **`additionalDomainNames`** - You can choose to listen on additional domains for each endpoint, however Solr will not register itself under these names.
  - **`useExternalAddress`** - Use the external address to advertise the SolrNode. If a domain name is required for the chosen external `method`, then the one provided in `domainName` will be used.
  - **`hideCommon`** - Do not externally expose the common service (one endpoint for all solr nodes).
  - **`hideNodes`** - Do not externally expose each node. (This cannot be set to `true` if the cloud is running across multiple kubernetes clusters)
  - **`nodePortOverride`** - Make the Node Service(s) override the podPort. This is only available for the `Ingress` external method. If `hideNodes` is set to `true`, then this option is ignored. If provided, his port will be used to advertise the Solr Node. \
  If `method: Ingress` and `hideNodes: false`, then this value defaults to `80` since that is the default port that ingress controllers listen on.

**Note:** Unless both `external.method=Ingress` and `external.hideNodes=false`, a headless service will be used to make each Solr Node in the statefulSet addressable.
If both of those criteria are met, then an individual ClusterIP Service will be created for each Solr Node/Pod.

## Zookeeper Reference

Solr Clouds require an Apache Zookeeper to connect to.

The Solr operator gives a few options.

- Connecting to an already running zookeeper ensemble via [connection strings](#zk-connection-info)
- [Spinning up a provided](#provided-instance) Zookeeper Ensemble in the same namespace via the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator)

#### Chroot

Both options below come with options to specify a `chroot`, or a ZNode path for solr to use as it's base "directory" in Zookeeper.
Before the operator creates or updates a StatefulSet with a given `chroot`, it will first ensure that the given ZNode path exists and if it doesn't the operator will create all necessary ZNodes in the path.
If no chroot is given, a default of `/` will be used, which doesn't require the existence check previously mentioned.
If a chroot is provided without a prefix of `/`, the operator will add the prefix, as it is required by Zookeeper.

### ZK Connection Info

This is an external/internal connection string as well as an optional chRoot to an already running Zookeeeper ensemble.
If you provide an external connection string, you do not _have_ to provide an internal one as well.

#### ACLs

The Solr Operator allows for users to specify ZK ACL references in their Solr Cloud CRDs.
The user must specify the name of a secret that resides in the same namespace as the cloud, that contains an ACL username value and an ACL password value.
This ACL must have admin permissions for the [chroot](#chroot) given.

The ACL information can be provided through an ADMIN acl and a READ ONLY acl.  
- Admin: `SolrCloud.spec.zookeeperRef.connectionInfo.acl`
- Read Only: `SolrCloud.spec.zookeeperRef.connectionInfo.readOnlyAcl`

All ACL fields are **required** if an ACL is used.

- **`secret`** - The name of the secret, in the same namespace as the SolrCloud, that contains the admin ACL username and password.
- **`usernameKey`** - The name of the key in the provided secret that stores the admin ACL username.
- **`usernameKey`** - The name of the key in the provided secret that stores the admin ACL password.

### Provided Instance

If you do not require the Solr cloud to run cross-kube cluster, and do not want to manage your own Zookeeper ensemble,
the solr-operator can manage Zookeeper ensemble(s) for you.

Using the [zookeeper-operator](https://github.com/pravega/zookeeper-operator), a new Zookeeper ensemble can be spun up for 
each solrCloud that has this option specified.

The startup parameter `zookeeper-operator` must be provided on startup of the solr-operator for this parameter to be available.

## Override Built-in Solr Configuration Files

The Solr operator deploys well-configured SolrCloud instances with minimal input required from human operators. 
As such, the operator installs various configuration files automatically, including `solr.xml` for node-level settings and `log4j2.xml` for logging. 
However, there may come a time when you need to override the built-in configuration files with custom settings.

In general, users can provide custom config files by providing a ConfigMap in the same namespace as the SolrCloud instance; 
all custom config files should be stored in the same user-provided ConfigMap under different keys.
Point your SolrCloud definition to a user-provided ConfigMap using the following structure:
```
spec:
  ...
  customSolrKubeOptions:
    configMapOptions:
      providedConfigMap: <Custom-ConfigMap-Here>
```

### Custom solr.xml

Solr pods load node-level configuration settings from `/var/solr/data/solr.xml`. 
This important configuration file gets created by the `cp-solr-xml` initContainer which bootstraps the `solr.home` directory on each pod before starting the main container.
The default `solr.xml` is mounted into the `cp-solr-xml` initContainer from a ConfigMap named `<INSTANCE>-solrcloud-configmap` (where `<INSTANCE>` is the name of your SolrCloud instance) created by the Solr operator.

_Note: The data in the default ConfigMap is not editable! Any changes to the `solr.xml` in the default ConfigMap created by the operator will be overwritten during the next reconcile cycle._

Many of the specific values in `solr.xml` can be set using Java system properties; for instance, the following setting controls the read timeout for the HTTP client used by Solr's `HttpShardHandlerFactory`:
```
<int name="socketTimeout">${socketTimeout:600000}</int>
```
The `${socketTimeout:600000}` syntax means pull the value from a Java system property named `socketTimeout` with default `600000` if not set.

You can set Java system properties using the `solrOpts` string in your SolrCloud definition, such as:
```
spec:
  solrOpts: -DsocketTimeout=300000
```
This same approach works for a number of settings in `solrconfig.xml` as well.

However, if you need to customize `solr.xml` beyond what can be accomplished with Java system properties, 
then you need to supply your own `solr.xml` in a ConfigMap in the same namespace where you deploy your SolrCloud instance.
Provide your custom XML in the ConfigMap using `solr.xml` as the key as shown in the example below:
```
---
kind: ConfigMap
apiVersion: v1
metadata:
  name: custom-solr-xml
data:
  solr.xml: |
    <?xml version="1.0" encoding="UTF-8" ?>
    <solr>
      ... CUSTOM CONFIG HERE ...
    </solr>
```
**Important: Your custom `solr.xml` must include `<int name="hostPort">${hostPort:0}</int>` as the operator relies on this element to set the port Solr pods advertise to ZooKeeper. If this element is missing, then your Solr pods will not be created.**

You can get the default `solr.xml` from a Solr pod as a starting point for creating a custom config using `kubectl cp` as shown in the example below:
```
SOLR_POD_ID=$(kubectl get pod -l technology=solr-cloud --no-headers -o custom-columns=":metadata.name" | head -1)
kubectl cp $SOLR_POD_ID:/var/solr/data/solr.xml ./custom-solr.xml
```
This copies the default config from the first Solr pod found in the namespace and names it `custom-solr.xml`. Customize the settings in `custom-solr.xml` as needed and then create a ConfigMap using YAML. 

_Note: Using `kubectl create configmap --from-file` scrambles the XML formatting, so we recommend defining the configmap YAML as shown above to keep the XML formatted properly._

Point your SolrCloud instance at the custom ConfigMap using:
```
  customSolrKubeOptions:
    configMapOptions:
      providedConfigMap: custom-solr-xml
```
_Note: If you set `providedConfigMap`, then the ConfigMap must include the `solr.xml` or `log4j2.xml` key, otherwise the SolrCloud will fail to reconcile._

#### Changes to Custom Config Trigger Rolling Restarts

The Solr operator stores the MD5 hash of your custom XML in the StatefulSet's pod spec annotations (`spec.template.metadata.annotations`). To see the current annotations for your Solr pods, you can do:
```
kubectl annotate pod -l technology=solr-cloud --list=true
```
If the custom `solr.xml` changes in the user-provided ConfigMap, then the operator triggers a rolling restart of Solr pods to apply the updated configuration settings automatically.

To summarize, if you need to customize `solr.xml`, provide your own version in a ConfigMap and changes made to the XML in the ConfigMap are automatically applied to your Solr pods.

### Custom Log Configuration

By default, the Solr Docker image configures Solr to load its log configuration from `/var/solr/log4j2.xml`. 
If you need to fine-tune the log configuration, then you can provide a custom `log4j2.xml` in a ConfigMap using the same basic process as described in the previous section for customizing `solr.xml`. If supplied, the operator overrides the log config using the `LOG4J_PROPS` env var.

As with custom `solr.xml`, the operator can track the MD5 hash of your `log4j2.xml` in the pod spec annotations to trigger a rolling restart if the log config changes. 
However, Log4j2 supports hot reloading of log configuration using the `monitorInterval` attribute on the root `<Configuration>` element. For more information on this, see: [Log4j Automatic Reconfiguration](https://logging.apache.org/log4j/2.x/manual/configuration.html#AutomaticReconfiguration). 
If your custom log config has a `monitorInterval` set, then the operator does not watch for changes to the log config and will not trigger a rolling restart if the config changes. 
Kubernetes will automatically update the file on each pod's filesystem when the data in the ConfigMap changes. Once Kubernetes updates the file, Log4j will pick up the changes and apply them without restarting the Solr pod.

If you need to customize both `solr.xml` and `log4j2.xml` then you need to supply both in the same ConfigMap using multiple keys as shown below:
```
---
kind: ConfigMap
apiVersion: v1
metadata:
  name: custom-solr-xml
data:
  log4j2.xml: |
    <?xml version="1.0" encoding="UTF-8"?>
    <Configuration monitorInterval="30">
     ... YOUR CUSTOM LOG4J CONFIG HERE ...
    </Configuration>


  solr.xml: |
    <?xml version="1.0" encoding="UTF-8" ?>
    <solr>
     ... YOUR CUSTOM SOLR XML CONFIG HERE ...
    </solr>
```