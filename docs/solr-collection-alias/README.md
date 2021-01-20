# Solr Collection Alias

The solr-operator supports the full lifecycle of standard aliases. Here is an example pointing an alias to 2 collections

```
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCollectionAlias
metadata:
  name: collection-alias-example
spec:
  solrCloud: example
  aliasType: standard
  collections:
    - example-collection-1
    - example-collection-2
```

Aliases can be useful when migrating from one collection to another without having to update application endpoint configurations.

Routed aliases are presently not supported