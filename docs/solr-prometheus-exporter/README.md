# Solr Prometheus Exporter

Solr metrics can be collected from solr clouds/standalone solr both residing within the kubernetes cluster and outside.
To use the Prometheus exporter, the easiest thing to do is just provide a reference to a Solr instance. That can be any of the following:
- The name and namespace of the Solr Cloud CRD
- The Zookeeper connection information of the Solr Cloud
- The address of the standalone Solr instance

You can also provide a custom Prometheus Exporter config, Solr version, and exporter options as described in the
[Solr ref-guide](https://lucene.apache.org/solr/guide/monitoring-solr-with-prometheus-and-grafana.html#command-line-parameters).

Note that a few of the official Solr docker images do not enable the Prometheus Exporter.
Versions `6.6` - `7.x` and `8.2` - `master` should have the exporter available. 

## Finding the Solr Cluster to monitor

The Prometheus Exporter supports metrics for both standalone solr as well as Solr Cloud.

### Cloud

You have two options for the prometheus exporter to find the zookeeper connection information that your Solr Cloud uses.

- Provide the name of a `SolrCloud` object in the same Kubernetes cluster, and optional namespace.
The Solr Operator will keep the ZK Connection info up to date from the SolrCloud object.  
This name can be provided at: `SolrPrometheusExporter.spec.solrRef.cloud.name`
- Provide explicit Zookeeper Connection info for the prometheus exporter to use.  
  This info can be provided at: `SolrPrometheusExporter.spec.solrRef.cloud.zkConnectionInfo`, with keys `internalConnectionString` and `chroot`

#### ACLs

The Prometheus Exporter can be set up to use ZK ACLs when connecting to Zookeeper.

If the prometheus exporter has been provided the name of a solr cloud, through `cloud.name`, then the solr operator will load up the ZK ACL Secret information found in the [SolrCloud spec](../solr-cloud/solr-cloud-crd.md#acls).
In order for the prometheus exporter to have visibility to these secrets, it must be deployed to the same namespace as the referenced SolrCloud or the same exact secrets must exist in both namespaces.

If explicit Zookeeper connection information has been provided, through `cloud.zkConnectionInfo`, then ACL information must be provided in the same section.
The ACL information can be provided through an ADMIN acl and a READ ONLY acl.  
- Admin: `SolrPrometheusExporter.spec.solrRef.cloud.zkConnectionInfo.acl`
- Read Only: `SolrPrometheusExporter.spec.solrRef.cloud.zkConnectionInfo.readOnlyAcl`

All ACL fields are **required** if an ACL is used.

- **`secret`** - The name of the secret, in the same namespace as the SolrCloud, that contains the admin ACL username and password.
- **`usernameKey`** - The name of the key in the provided secret that stores the admin ACL username.
- **`usernameKey`** - The name of the key in the provided secret that stores the admin ACL password.

### Standalone

The Prometheus Exporter can be setup to scrape a standalone Solr instance.
In order to use this functionality, use the following spec field:

`SolrPrometheusExporter.spec.solrRef.standalone.address`