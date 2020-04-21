# Solr Collections

Solr-operator can manage the creation, deletion and modification of Solr collections. 

Collection creation requires a Solr Cloud to apply against. Presently, SolrCollection supports both implicit and compositeId router types, with some of the basic configuration options including `autoAddReplicas`. 

Create an example set of collections against on the "example" solr cloud

```bash
$ cat example/test_solrcollection.yaml

apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCollection
metadata:
  name: example-collection-1
spec:
  solrCloud: example
  collection: example-collection
  routerName: compositeId
  autoAddReplicas: false
  numShards: 2
  replicationFactor: 1
  maxShardsPerNode: 1
  collectionConfigName: "_default"
---
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCollection
metadata:
  name: example-collection-2-compositeid-autoadd
spec:
  solrCloud: example
  collection: example-collection-2
  routerName: compositeId
  autoAddReplicas: true
  numShards: 2
  replicationFactor: 1
  maxShardsPerNode: 1
  collectionConfigName: "_default"
---
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCollection
metadata:
  name: example-collection-3-implicit
spec:
  solrCloud: example
  collection: example-collection-3-implicit
  routerName: implicit
  autoAddReplicas: true
  numShards: 2
  replicationFactor: 1
  maxShardsPerNode: 1
  shards: "fooshard1,fooshard2"
  collectionConfigName: "_default"
```

```bash
$ kubectl apply -f examples/test_solrcollections.yaml
```