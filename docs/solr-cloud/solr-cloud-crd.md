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

The SolrCloud CRD provides users the ability to define how Pod updates should be managed, through `SolrCloud.Spec.updateStrategy`.
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

These options are configured under `spec.zookeeperRef`

#### Chroot

Both options below come with options to specify a `chroot`, or a ZNode path for solr to use as it's base "directory" in Zookeeper.
Before the operator creates or updates a StatefulSet with a given `chroot`, it will first ensure that the given ZNode path exists and if it doesn't the operator will create all necessary ZNodes in the path.
If no chroot is given, a default of `/` will be used, which doesn't require the existence check previously mentioned.
If a chroot is provided without a prefix of `/`, the operator will add the prefix, as it is required by Zookeeper.

### ZK Connection Info

This is an external/internal connection string as well as an optional chRoot to an already running Zookeeeper ensemble.
If you provide an external connection string, you do not _have_ to provide an internal one as well.

Under `spec.zookeeperRef`:

- **`connectionInfo`**
  - **`externalConnectionString`** - The ZK connection string to the external Zookeeper cluster, e.g. `zoo1:2181`
  - **`chroot`** - The chroot to use for the cluster

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

## Enable TLS Between Solr Pods

A common approach to securing traffic to your Solr cluster is to perform **TLS termination** at the Ingress and leave all traffic between Solr pods un-encrypted.
However, depending on how you expose Solr on your network, you may also want to encrypt traffic between Solr pods.
The Solr operator provides **optional** configuration settings to enable TLS for encrypting traffic between Solr pods.

Enabling TLS for Solr is a straight-forward process once you have a [**PKCS12 keystore**]((https://en.wikipedia.org/wiki/PKCS_12)) containing an [X.509](https://en.wikipedia.org/wiki/X.509) certificate and private key; as of Java 8, PKCS12 is the default keystore format supported by the JVM.

There are two basic use cases supported by the Solr operator. First, you can use cert-manager to issue a certificate and store the resulting PKCS12 keystore in a Kubernetes TLS secret. 
Alternatively, you can create the TLS secret manually from a certificate obtained by some other means. In both cases, you simply point your SolrCloud CRD to the resulting TLS secret and corresponding keystore password secret.

### Use cert-manager to issue the certificate

[cert-manager](https://cert-manager.io/docs/) is a popular Kubernetes controller for managing TLS certificates, including renewing certificates prior to expiration. 
One of the primary benefits of cert-manager is it supports pluggable certificate `Issuer` implementations, including a self-signed Issuer for local development and an [ACME compliant](https://tools.ietf.org/html/rfc8555) Issuer for working with services like [Let’s Encrypt](https://letsencrypt.org/).

If you already have a TLS certificate you want to use for Solr, then you don't need cert-manager and can skip down to [I already have a TLS Certificate](#-Already-Have-a-TLS-Certificate) later in this section.
If you do not have a TLS certificate, then we recommend installing **cert-manager** as it makes working with TLS in Kubernetes much easier.

#### Install cert-manager

Given its popularity, cert-manager may already be installed in your Kubernetes cluster. To check if `cert-manager` is already installed, do:
```
kubectl get crds -l app.kubernetes.io/instance=cert-manager
```
If installed, you should see the following cert-manager related CRDs:
```
certificaterequests.cert-manager.io
certificates.cert-manager.io
challenges.acme.cert-manager.io
clusterissuers.cert-manager.io
issuers.cert-manager.io
orders.acme.cert-manager.io
```

If not intalled, use Helm to install it into the `cert-manager` namespace:
```
if ! helm repo list | grep -q "https://charts.jetstack.io"; then
  helm repo add jetstack https://charts.jetstack.io
  helm repo update
fi

kubectl create ns cert-manager
helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --version v1.1.0 \
  --set installCRDs=true
``` 
You’ll need admin privileges to install the CRDs in a shared K8s cluster, so work with your friendly Kubernetes admin to install if needed (most likely cert-manager will already be installed).
Refer to the [cert-manager Installation](https://cert-manager.io/docs/installation/kubernetes/) instructions for more information.

#### Create cert-manager Certificate

Once cert-manager is installed, you need to create an `Issuer` or `ClusterIssuer` CRD and then request a certificate using a [Certificate CRD](https://cert-manager.io/docs/usage/certificate/).
Refer to the [cert-manager docs](https://cert-manager.io/docs/) on how to define a certificate.

Certificate Issuers are typically platform specific. For instance, on GKE, to create a Let’s Encrypt Issuer you need a service account with various cloud DNS permissions granted for DNS01 challenges to work, see: https://cert-manager.io/docs/configuration/acme/dns01/google/.

The DNS names in your certificate should match the Solr addressability settings in your SolrCloud CRD. For instance, if your SolrCloud CRD uses the following settings:
```
  solrAddressability:
    external:
      domainName: k8s.solr.cloud
``` 
Then your certificate needs the following domains specified:
```
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  ...
spec:
  dnsNames:
  - '*.k8s.solr.cloud'
  - k8s.solr.cloud
```
The wildcard DNS name will cover all SolrCloud nodes such as `<NS>-solrcloud-1.k8s.solr.cloud`.

Also, when requesting your certificate, keep in mind that internal DNS names in Kubernetes are not valid for public certificates. 
For instance `<svc>.<namespace>.svc.cluster.local` is internal to Kubernetes and certificate issuer services like LetsEncrypt 
will not generate a certificate for K8s internal DNS names (you'll get errors during certificate issuing).

Another benefit is cert-manager can create a [PKCS12](https://cert-manager.io/docs/release-notes/release-notes-0.15/#general-availability-of-jks-and-pkcs-12-keystores) keystore automatically when issuing a `Certificate`, 
which allows the Solr operator to mount the keystore directly on our Solr pods. Ensure your certificate instance requests **pkcs12 keystore** gets created using config similar to the following:
```
  keystores:
    pkcs12:
      create: true
      passwordSecretRef:
        key: password-key
        name: pkcs12-password-secret
```
_Note: the example structure above goes in your certificate CRD YAML, not SolrCloud._

You need to create the keystore secret (e.g. `pkcs12-password-secret`) in the same namespace before requesting the certificate, see: https://cert-manager.io/docs/reference/api-docs/#cert-manager.io/v1.PKCS12Keystore.
Although a keystore password is not required for PKCS12, **cert-manager** requires a password when requesting a `pkcs12` keystore for your certificate.
Moreover, most JVMs require a password for pkcs12 keystores, not supplying a password typically results in errors like the following:
```
Caused by: java.security.UnrecoverableKeyException: Get Key failed: null
	at java.base/sun.security.pkcs12.PKCS12KeyStore.engineGetKey(Unknown Source)
	at java.base/sun.security.util.KeyStoreDelegator.engineGetKey(Unknown Source)
	at java.base/java.security.KeyStore.getKey(Unknown Source)
	at java.base/sun.security.ssl.SunX509KeyManagerImpl.<init>(Unknown Source)
```
Consequently, the Solr operator requires you to use a non-null password for your keystore. 

Here's an example of how to use cert-manager to generate a self-signed certificate:
```
---
apiVersion: v1
kind: Secret
metadata:
  name: pkcs12-password-secret
data:
  password-key: SOME_PASSWORD_HERE

---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}

---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: selfsigned-cert
spec:
  subject:
    organizations: ["dev"]
  dnsNames:
    - localhost
    - dev-dev-solrcloud.ing.local.domain
    - "*.ing.local.domain"
  secretName: dev-selfsigned-cert-tls
  issuerRef:
    name: selfsigned-issuer
  keystores:
    pkcs12:
      create: true
      passwordSecretRef:
        key: password-key
        name: pkcs12-password-secret
```

Once created, simply point the SolrCloud deployment at the TLS and keystore password secrets, e.g.
```
spec:
  ... other SolrCloud CRD settings ...

  solrTLS:
    keyStorePasswordSecret:
      name: pkcs12-password-secret
      key: password-key
    pkcs12Secret:
      name: selfsigned-cert
      key: keystore.p12
```
_Note: when using self-signed certificates, you'll have to configure HTTP client libraries to skip hostname and CA verification._

### I already have a TLS Certificate

Users may bring their own cert stored in a `kubernetes.io/tls` secret; for this use case, cert-manager is not required. 
There are many ways to get a certificate, such as from the GKE managed certificate process or from a CA directly. 
Regardless of how you obtain a Certificate, it needs to be stored in a [Kubernetes TLS secret](https://kubernetes.io/docs/concepts/configuration/secret/#tls-secrets) 
that contains a `tls.crt` file (x.509 certificate with a public key and info about the issuer) and a `tls.key` file (the private key).

Ideally, the TLS secret will also have a `pkcs12` keystore. 
If the supplied TLS secret does not contain a `keystore.p12` key, then the Solr operator creates an `initContainer` on the StatefulSet to generate the keystore from the TLS secret using the following command:
```
openssl pkcs12 -export -in tls.crt -inkey tls.key -out keystore.p12 -passout pass:${SOLR_SSL_KEY_STORE_PASSWORD}"
```
_The `initContainer` uses the main Solr image as it has `openssl` installed._

Configure the SolrCloud deployment to point to the user-provided keystore and TLS secrets:
```
spec:
  ... other SolrCloud CRD settings ...

  solrTLS:
    keyStorePasswordSecret:
      name: pkcs12-keystore-manual
      key: password-key
    pkcs12Secret:
      name: pkcs12-keystore-manual
      key: keystore.p12
```

### Ingress

The Solr operator may create an Ingress for exposing Solr pods externally. When TLS is enabled, the operator adds the following annotation and TLS settings to the Ingress manifest, such as:
```
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: HTTPS
spec:
  rules:
    ...
  tls:
  - secretName: my-selfsigned-cert-tls
```

### Certificate Renewal and Rolling Restarts

cert-manager automatically handles certificate renewal. From the docs:

> The default duration for all certificates is 90 days and the default renewal windows is 30 days. This means that certificates are considered valid for 3 months and renewal will be attempted within 1 month of expiration.
>          https://docs.cert-manager.io/en/release-0.8/reference/certificates.html

However, this only covers updating the underlying TLS secret and mounted secrets in each Solr pod do get updated on the filesystem, see: https://kubernetes.io/docs/concepts/configuration/secret/#mounted-secrets-are-updated-automatically. 
However, the JVM only reads key and trust stores once during initialization and does not reload them if they change. Thus, we need to recycle the Solr container in each pod to pick up the updated keystore.

The operator tracks the MD5 hash of the `tls.crt` from the TLS secret in an annotation on the StatefulSet pod spec so that when the TLS secret changes, it will trigger a rolling restart of the affected Solr pods.
The operator guards this behavior with an **opt-in** flag `restartOnTLSSecretUpdate` as some users may not want to restart Solr pods when the TLS secret holding the cert changes and may instead choose to restart the pods during a maintenance window (presumably before the certs expire).
```
spec:
  ... other SolrCloud CRD settings ...

  solrTLS:
    restartOnTLSSecretUpdate: true
    ...

```

### Misc Config Settings for TLS Enabled Solr

Although not required, we recommend setting the `commonServicePort` and `nodePortOverride` to `443` instead of the default port `80` under `solrAddressability` to avoid confusion when working with `https`. 
```
spec:
  ... other SolrCloud CRD settings ...

  solrAddressability:
    commonServicePort: 443
    external:
      nodePortOverride: 443

```

#### Prometheus Exporter

If you're relying on a self-signed certificate (or any certificate that requires importing the CA into the Java trust store) for Solr pods, then the Prometheus Exporter will not be able to make requests for metrics. 
You'll need to duplicate your TLS config from your SolrCloud CRD definition to your Prometheus exporter CRD definition as shown in the example below:
```
  solrReference:
    cloud:
      name: "dev"
    solrTLS:
      restartOnTLSSecretUpdate: true
      keyStorePasswordSecret:
        name: pkcs12-password-secret
        key: password-key
      pkcs12Secret:
        name: dev-selfsigned-cert-tls
        key: keystore.p12
```
_This only applies to the SolrJ client the exporter uses to make requests to your TLS-enabled Solr pods and does not enable HTTPS for the exporter service._

#### Public / Private Domain Names

If your Solr pods use Kubernetes internal domain names, such as `<cloud>-solrcloud-<oridinal>.<ns>` or 
`<cloud>-solrcloud-<oridinal>.<ns>.svc.cluster.local` then you **cannot** request a certificate from a service like LetsEncrypt. 
You'll receive an error like (from the cert-manager controller pod logs):
```
   Cannot issue for \"*.<ns>.svc.cluster.local\": Domain name does not end with a valid public suffix (TLD)"
```
This is policy enforced by trusted certificate authorities, see: https://www.digicert.com/kb/advisories/internal-names.htm.
Intuitively, this makes sense because services like LetsEncrypt cannot determine if you own a private domain because they cannot reach it from the Internet. 

Some CA's provide TLS certificates for private domains but that topic is beyond the scope of the Solr operator.
You may want to use a self-signed certificate for internal traffic and then a public certificate for your Ingress.
Alternatively, you can choose to expose Solr pods with an external name using SolrCloud `solrAddressability` settings:
```
kind: SolrCloud
metadata:
  name: search
spec:
  ... other SolrCloud CRD settings ...

  solrAddressability:    
    commonServicePort: 443
    external:
      nodePortOverride: 443
      domainName: k8s.solr.cloud
      method: Ingress
      useExternalAddress: true
```
The example settings above will result in your Solr pods getting names like: `<ns>-search-solrcloud-0.k8s.solr.cloud` 
which you can request TLS certificates from LetsEncrypt assuming you own the `k8s.solr.cloud` domain.

## Solr Security: Authentication and Authorization

All well-configured Solr clusters should enforce users to authenticate, even for read-only operations. Even if you want
to allow anonymous query requests from unknown users, you should make this explicit using Solr's rule-based authorization
plugin. The Solr operator can bootstrap a default security configuration for your SolrCloud during initialization. As such,
there is really no reason to deploy an unsecured SolrCloud cluster when using the Solr operator.

For background on Solr security, please refer to the [Reference Guide](https://lucene.apache.org/solr/guide) for your version of Solr.

In general, you have two basic options for configuring Solr security with the Solr operator:
1. Let the Solr operator bootstrap the `security.json` to configure basic authentication for Solr.
2. Supply your own `security.json` to Solr, which must define a user account that the operator can use to make API requests to secured Solr pods.

If you choose option #2, then you need to provide the credentials to the Solr operator using a Kubernetes [Basic Authentication Secret](https://kubernetes.io/docs/concepts/configuration/secret/#basic-authentication-secret).
With option #1, the operator creates the Basic Authentication Secret for you.

### Option 1: Bootstrap Security

The easiest way to get started with Solr security is to have the operator bootstrap a `security.json` (stored in ZK) as part of the initial deployment process.
To activate this feature, add the following configuration to your SolrCloud CRD definition YAML:
```
spec:
  ...
  solrSecurity:
    authenticationType: Basic
```

Once the cluster is up, you'll need the `admin` user password to login to the Solr Admin UI:
```
kubectl get secret <CLOUD>-solrcloud-security-bootstrap -o jsonpath='{.data.admin}' | base64 --decode
```
_where `<CLOUD>` is the name of your SolrCloud_

The `admin` user will have a random password generated by the operator during `security.json` bootstrapping. 
Once `security.json` is bootstrapped, the operator will not update it! You're expected to use the `admin` user to access the Security API to make further changes. 
It's strictly a bootstrapping operation. In addition to the `admin` user, the operator defines a `solr` user, which has basic read access to Solr resources.

The username (`k8s-oper`) and password used by the operator to make API requests to Solr are stored in a separate secret of type `kubernetes.io/basic-auth`
with name `<CLOUD>-solrcloud-basic-auth`. The `k8s-oper` user is configured with read-only access to a minimal set of endpoints.

The default `security.json` created by the operator is shown below, except the passwords for each user will be randomized for every SolrCloud you create:
```
{
  "authentication": {
    "blockUnknown": false,
    "class": "solr.BasicAuthPlugin",
    "credentials": {
      "admin": "...",
      "k8s-oper": "...",
      "solr": "..."
    },
    "realm": "Solr Basic Auth",
    "forwardCredentials": false
  },
  "authorization": {
    "class": "solr.RuleBasedAuthorizationPlugin",
    "user-role": {
      "admin": [
        "admin",
        "k8s"
      ],
      "k8s-oper": [
        "k8s"
      ],
      "solr": [
        "users",
        "k8s"
      ]
    },
    "permissions": [
      {
        "name": "k8s-probe-0",
        "role": null,
        "collection": null,
        "path": "/admin/info/system"
      },
      {
        "name": "k8s-status",
        "role": "k8s",
        "collection": null,
        "path": "/admin/collections"
      },
      {
        "name": "k8s-metrics",
        "role": "k8s",
        "collection": null,
        "path": "/admin/metrics"
      },
      {
        "name": "k8s-ping",
        "role": "k8s",
        "collection": "*",
        "path": "/admin/ping"
      },
      {
        "name": "all",
        "role": [
          "admin",
          "users"
        ]
      },
      {
        "name": "read",
        "role": [
          "admin",
          "users"
        ]
      },
      {
        "name": "update",
        "role": [
          "admin"
        ]
      },
      {
        "name": "security-read",
        "role": "admin"
      },
      {
        "name": "security-edit",
        "role": "admin"
      }
    ]
  }
}
```

#### Liveness and Readiness Probes

We recommend configuring Solr to allow un-authenticated access over HTTP to the probe endpoint(s) and the bootstrapped `security.json` does this for you automatically. 
However, if you want secure the probe endpoints, then you need to set `probesRequireAuth: true` as shown below:
```
spec:
  ...
  solrSecurity:
    authenticationType: Basic
    probesRequireAuth: true
```
When `probesRequireAuth` is set to `true`, the liveness and readiness probes execute a command instead of using HTTP. 
The operator configures a command instead of setting the `Authorization` header for the HTTP probes, as that would require a restart of all pods if the password changes. 
With a command, we can load the username and password from files that come from a secret. Kubernetes will update the secret file when the secret changes automatically.

### Option 2: User-provided Basic Auth Secret

Alternatively, if users want full control over their cluster's security config, then they can provide a secret that holds the credentials for the user they want the operator to make API requests as:
```
spec:
  ...
  solrSecurity:
    authenticationType: Basic
    basicAuthSecret: user-provided-secret
```

The supplied secret must be of type [Basic Authentication Secret](https://kubernetes.io/docs/concepts/configuration/secret/#basic-authentication-secret) and define both a `username` and `password`.
With this config, the operator will make API request to secure Solr as the specified user. If users supply their own basic auth secret, then the operator *does not* bootstrap the `security.json`, that is left to the user to supply to Solr. 
Users need to ensure their `security.json` contains the user supplied in the `basicAuthSecret` with read access to:
```
/admin/info/system
/admin/collections
/admin/metrics
/admin/ping (for collection="*")
```
_Tip: see the authorization rules defined by the default `security.json` as a guide for configuring access for the operator user_

### Prometheus Exporter with Basic Auth

If you enable basic auth for your SolrCloud cluster, then you need to point the Prometheus exporter at the basic auth secret containing the credentials for making API requests to `/admin/metrics` and `/admin/ping` for all collections.

```
spec:
  solrReference:
    basicAuthSecret: user-provided-secret
```

If you chose option #1 to have the operator bootstrap `security.json` for you, then the name of the secret will be:
`<CLOUD>-solrcloud-basic-auth`.