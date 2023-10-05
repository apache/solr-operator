# Changes for new minimum Solr Versions

This is a list of improvements that can be made to the Solr Operator when it is guaranteed that all Solrs will be at least a certain version.

So when upgrading the minimum supported Solr Version for the operator, we can then go ahead and make all improvements that align with versions <= the new minimum supported version.

## 8.x

### 8.11

- SOLR-7642: Solr will create a chroot if necessary using the ZK_CREATE_CHROOT envVar

## 9.x

### 9.0

- SOLR-14957: Prometheus exporter bin is now in the PATH for the Solr docker image
- SOLR-14957: The default Prometheus exporter config is now on the classpath, and it does not need to be provided on startup.
- SOLR-15914: Solr Modules can be included via an Environment variable, no need to use `sharedLib`
- SOLR-9575: No need to pre-fill the solr.xml in SOLR_HOME (combined with SOLR-14999)

## Future Wishlist

- Have a bin/solr command to healthcheck the Solr Node (possibly split between `live` and `ready`).
  This command would need to support ZKACLs as well as SSL and basicAuth/jwt. (related to SOLR-15199)
