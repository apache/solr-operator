# Ethos Enhacement
## Changes
- Added new CRD element taintedSolrPodsSelector to define ethos upgrade pod label
- Added solr cloud reconcile logic to detect pod label and trigger safely reschedule of the pod
- Enhanced solr helm chart for new CRD element taintedSolrPodsSelector

## Example Use
### Define Tainted Label in Solr Cloud Helm Values
```sh
taintedSolrPodsSelector:
  matchExpressions:
    - key: shredder.ethos.adobe.net/allow-eviction
      operator: In
      values:
      - "false"
```
### Add Pod Label
```sh
kubectl label pod -l technology=solr-cloud shredder.ethos.adobe.net/allow-eviction=false
```
## Build
```sh
make build
REPOSITORY=zhaoqi0406 TAG=v0.5.1-ethos make docker-build docker-push
```
## Deployment 
```sh
kubectl create -f https://solr.apache.org/operator/downloads/crds/v0.5.1/all-with-dependencies.yaml

kubectl replace -f solr-operator/helm/solr-operator/crds/crds.yaml

helm install solr-operator apache-solr/solr-operator --version 0.5.1 --set image.repository=zhaoqi0406/solr-operator --set image.tag=v0.5.1-ethos

helm install example-solr apache-solr/solr --version 0.5.1 \
  --set image.tag=8.3 \
  --set solrOptions.javaMemory="-Xms300m -Xmx300m" \
  --set addressability.external.method=Ingress \
  --set addressability.external.domainName="ing.local.domain" \
  --set addressability.external.useExternalAddress="true" \
  --set ingressOptions.ingressClassName="nginx" \
  --set updateStrategy.managedUpdate.maxPodsUnavailable="2" \
  --set updateStrategy.method="Managed" \
  --values <pod label value file>
```





