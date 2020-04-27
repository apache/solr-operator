# The SolrCloud CRD

The SolrCloud CRD allows users to spin up a Solr cloud in a very configurable way.
Those configuration options are layed out on this page.

## Zookeeper Reference

Solr Clouds require an Apache Zookeeper to connect to.

The Solr operator gives a few options.

**Note** - Both options below come with options to specify a `chroot`, or a ZNode path for solr to use as it's base "directory" in Zookeeper. Before the operator creates or updates a StatefulSet with a given `chroot`, it will first ensure that the given ZNode path exists and if it doesn't the operator will create all necessary ZNodes in the path. If no chroot is given, a default of `/` will be used, which doesn't require the existence check previously mentioned. If a chroot is provided without a prefix of `/`, the operator will add the prefix, as it is required by Zookeeper.

### ZK Connection Info

This is an external/internal connection string as well as an optional chRoot to an already running Zookeeeper ensemble.
If you provide an external connection string, you do not _have_ to provide an internal one as well.

### Provided Instance

If you do not require the Solr cloud to run cross-kube cluster, and do not want to manage your own Zookeeper ensemble,
the solr-operator can manage Zookeeper ensemble(s) for you.

#### Zookeeper

Using the [zookeeper-operator](https://github.com/pravega/zookeeper-operator), a new Zookeeper ensemble can be spun up for 
each solrCloud that has this option specified.

The startup parameter `zookeeper-operator` must be provided on startup of the solr-operator for this parameter to be available.

#### Zetcd

Using [etcd-operator](https://github.com/coreos/etcd-operator), a new Etcd ensemble can be spun up for each solrCloud that has this option specified.
A [Zetcd](https://github.com/etcd-io/zetcd) deployment is also created so that Solr can interact with Etcd as if it were a Zookeeper ensemble.

The startup parameter `etcd-operator` must be provided on startup of the solr-operator for this parameter to be available.
