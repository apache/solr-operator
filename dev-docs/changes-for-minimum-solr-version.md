# Changes for new minimum Solr Versions

This is a list of improvements that can be made to the Solr Operator when it is guaranteed that all Solrs will be at least a certain version.

So when upgrading the minimum supported Solr Version for the operator, we can then go ahead and make all improvements that align with versions <= the new minimum supported version.

## 8.x

## 8.1

- Adding `<int name="maxBooleanClauses">${solr.max.booleanClauses:1024}</int>` to the default `solr.xml`.

## 8.5

- Adding `<str name="sharedLib">${solr.sharedLib:}</str>` to the default `solr.xml`.

## 8.6

- Adding `<str name="allowPaths">${solr.allowPaths:}</str>` to the default `solr.xml`.

## 8.7

- Adding `<metrics enabled="${metricsEnabled:true}"/>` to the default `solr.xml`.