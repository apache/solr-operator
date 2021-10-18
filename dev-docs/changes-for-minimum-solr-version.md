# Changes for new minimum Solr Versions

This is a list of improvements that can be made to the Solr Operator when it is guaranteed that all Solrs will be at least a certain version.

So when upgrading the minimum supported Solr Version for the operator, we can then go ahead and make all improvements that align with versions <= the new minimum supported version.

## 8.x

### 8.0

- SOLR-11126: Used improved healthcheck handler (`/solr/admin/health`)

### 8.1

- SOLR-13336: Adding `<int name="maxBooleanClauses">${solr.max.booleanClauses:1024}</int>` to the default `solr.xml`.

### 8.3

- SOLR-13773: Adding SOLR_HEAP, SOLR_JAVA_MEM, GC_TUNE envVar options for the Prometheus Exporter

### 8.5

- SOLR-14281: Adding `<str name="sharedLib">${solr.sharedLib:}</str>` to the default `solr.xml`.

### 8.6

- SOLR-14561: Adding `<str name="allowPaths">${solr.allowPaths:}</str>` to the default `solr.xml`.

### 8.7

- SOLR-14914: Adding `<metrics enabled="${metricsEnabled:true}"/>` to the default `solr.xml`.

### 8.8

- SOLR-14955: Many of the prometheus exporter options can be set via EnvVars now
- SOLR-14999: Use SOLR_PORT_ADVERTISE for the hostPort information, instead of using a custom option in the solr.xml
  When we make this upgrade, we will not need to use a custom solr.xml unless the user has specified custom options, such as backup repositories.

### 8.11

- SOLR-7642: Solr will create a chroot if necessary using the ZK_CREATE_CHROOT envVar

## 9.x

- SOLR-14957: Prometheus exporter bin is now in the PATH for the Solr docker image

## Future Wishlist

- Have a bin/solr command to healthcheck the Solr Node (possibly split between `live` and `ready`).
  This command would need to support ZKACLs as well as SSL and basicAuth/jwt. (related to SOLR-15199)
