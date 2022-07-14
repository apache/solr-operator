<!--
    Licensed to the Apache Software Foundation (ASF) under one or more
    contributor license agreements.  See the NOTICE file distributed with
    this work for additional information regarding copyright ownership.
    The ASF licenses this file to You under the Apache License, Version 2.0
    the "License"); you may not use this file except in compliance with
    the License.  You may obtain a copy of the License at

        http://www.apache.org/licenses/LICENSE-2.0

    Unless required by applicable law or agreed to in writing, software
    distributed under the License is distributed on an "AS IS" BASIS,
    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    See the License for the specific language governing permissions and
    limitations under the License.
 -->

# Solr Prometheus Exporter

Solr metrics can be collected from solr clouds/standalone solr both residing within the kubernetes cluster and outside.
To use the Prometheus exporter, the easiest thing to do is just provide a reference to a Solr instance. That can be any of the following:
- The name and namespace of the Solr Cloud CRD
- The Zookeeper connection information of the Solr Cloud
- The address of the standalone Solr instance

You can also provide a custom Prometheus Exporter config, Solr version, and exporter options as described in the
[Solr ref-guide](https://solr.apache.org/guide/monitoring-solr-with-prometheus-and-grafana.html#command-line-parameters).

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

If `SolrPrometheusExporter.spec.solrRef.cloud.name` is used and no image information is passed via `SolrPrometheusExporter.spec.image.*` options, then the Prometheus Exporter will use the same image as the SolrCloud it is listening to.
If any `SolrPrometheusExporter.spec.image.*` option is provided, then the Prometheus Exporter will use its own image.

#### ACLs
_Since v0.2.7_

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
- **`passwordKey`** - The name of the key in the provided secret that stores the admin ACL password.

### Standalone

The Prometheus Exporter can be setup to scrape a standalone Solr instance.
In order to use this functionality, use the following spec field:

`SolrPrometheusExporter.spec.solrRef.standalone.address`


### Solr TLS
_Since v0.3.0_

If you're relying on a self-signed certificate (or any certificate that requires importing the CA into the Java trust store) for Solr pods, then the Prometheus Exporter will not be able to make requests for metrics.
You'll need to duplicate your TLS config from your SolrCloud CRD definition to your Prometheus exporter CRD definition as shown in the example below:

```yaml
spec:
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

**This only applies to the SolrJ client the exporter uses to make requests to your TLS-enabled Solr pods and does not enable HTTPS for the exporter service.**

#### Mounted TLS Directory
_Since v0.4.0_

You can use the `spec.solrReference.solrTLS.mountedTLSDir.path` to point to a directory containing certificate files mounted by an external agent or CSI driver.

### Prometheus Exporter with Basic Auth
_Since v0.3.0_

If you enable basic auth for your SolrCloud cluster, then you need to point the Prometheus exporter at the basic auth secret containing the credentials for making API requests to `/admin/metrics` and `/admin/ping` for all collections.

```yaml
spec:
  solrReference:
    basicAuthSecret: user-provided-secret
```
If you chose option #1 to have the operator bootstrap `security.json` for you, then the name of the secret will be:
`<CLOUD>-solrcloud-basic-auth`. If you chose option #2, then pass the same name that you used for your SolrCloud CRD instance.

This user account will need access to the following endpoints in Solr:
```json
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
```

For more details on configuring Solr security with the operator, see [Authentication and Authorization](../solr-cloud/solr-cloud-crd.md#authentication-and-authorization)

## Prometheus Stack

In this section, we'll walk through how to use the Prometheus exporter with the [Prometheus Stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack).

The Prometheus Stack provides all the services you need for monitoring Kubernetes applications like Solr and is the recommended way of deploying Prometheus and Grafana.

### Install Prometheus Stack

Begin by installing the Prometheus Stack in the `monitoring` namespace with Helm release name `mon`:
```bash
MONITOR_NS=monitoring
PROM_OPER_REL=mon

kubectl create ns ${MONITOR_NS}

# see: https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack
if ! helm repo list | grep -q "https://prometheus-community.github.io/helm-charts"; then
  echo -e "\nAdding the prometheus-community repo to helm"
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo add stable https://charts.helm.sh/stable
  helm repo update
fi

helm upgrade --install ${PROM_OPER_REL} prometheus-community/kube-prometheus-stack \
--namespace ${MONITOR_NS} \
--set kubeStateMetrics.enabled=false \
--set nodeExporter.enabled=false
```
_Refer to the Prometheus stack documentation for detailed instructions._

Verify you have Prometheus / Grafana pods running in the `monitoring` namespace:
```bash
kubectl get pods -n monitoring
```

### Deploy Prometheus Exporter for Solr Metrics

Next, deploy a Solr Prometheus exporter for the SolrCloud you want to capture metrics from in the namespace where you're running SolrCloud, not in the `monitoring` namespace. 
For instance, the following example creates a Prometheus exporter named `dev-prom-exporter` for a SolrCloud named `dev` deployed in the `dev` namespace:
```yaml
apiVersion: solr.apache.org/v1beta1
kind: SolrPrometheusExporter
metadata:
  name: dev-prom-exporter
spec:
  customKubeOptions:
    podOptions:
      resources:
        requests:
          cpu: 300m
          memory: 900Mi
  solrReference:
    cloud:
      name: "dev"
  numThreads: 6
```

Look at the logs for your exporter pod to ensure it is running properly (notice we're using a label filter vs. addressing the pod by name):
```bash
kubectl logs -l solr-prometheus-exporter=dev-prom-exporter
```
You should see some log messages that look similar to:
```
INFO  - <timestamp>; org.apache.solr.prometheus.collector.SchedulerMetricsCollector; Beginning metrics collection
INFO  - <timestamp>; org.apache.solr.prometheus.collector.SchedulerMetricsCollector; Completed metrics collection
```

You can also see the metrics that are exported by the pod by opening a port-forward to the exporter pod and hitting port 8080 with cURL:
```bash
kubectl port-forward $(kubectl get pod -l solr-prometheus-exporter=dev-prom-exporter --no-headers -o custom-columns=":metadata.name") 8080

curl http://localhost:8080/metrics
```

#### Customize Prometheus Exporter Config
_Since v0.3.0_

Each Solr pod exposes metrics as JSON from the `/solr/admin/metrics` endpoint. To see this in action, open a port-forward to a Solr pod and send a request to `http://localhost:8983/solr/admin/metrics`.

The Prometheus exporter requests metrics from each pod and then extracts the desired metrics using a series of [jq](https://stedolan.github.io/jq/) queries against the JSON returned by each pod.

By default, the Solr operator configures the exporter to use the config from `/opt/solr/contrib/prometheus-exporter/conf/solr-exporter-config.xml`.

If you need to customize the metrics exposed to Prometheus, you'll need to provide a custom config XML via a ConfigMap and then configure the exporter CRD to point to it.

For instance, let's imagine you need to expose a new metric to Prometheus. Start by pulling the default config from the exporter pod using:
```bash
EXPORTER_POD_ID=$(kubectl get pod -l solr-prometheus-exporter=dev-prom-exporter --no-headers -o custom-columns=":metadata.name"`)

kubectl cp $EXPORTER_POD_ID:/opt/solr/contrib/prometheus-exporter/conf/solr-exporter-config.xml ./solr-exporter-config.xml
```
Create a ConfigMap with your customized XML config under the `solr-prometheus-exporter.xml` key.
```yaml
apiVersion: v1
data:
  solr-prometheus-exporter.xml: |
    <?xml version="1.0" encoding="UTF-8" ?>
    <config>
    ... YOUR CUSTOM CONFIG HERE ...
    </config>
kind: ConfigMap
metadata:
  name: custom-exporter-xml
```
_Note: Using `kubectl create configmap --from-file` scrambles the XML formatting, so we recommend defining the configmap YAML as shown above to keep the XML formatted properly._

Point to the custom ConfigMap in your Prometheus exporter definition using:
```yaml
spec:
  customKubeOptions:
    configMapOptions:
      providedConfigMap: custom-exporter-xml
``` 
The ConfigMap needs to be defined in the same namespace where you're running the exporter.

The Solr operator automatically triggers a restart of the exporter pods whenever the exporter config XML changes in the ConfigMap.

#### Solr Prometheus Exporter Service
The Solr operator creates a K8s `ClusterIP` service for load-balancing across exporter pods; there will typically only be one active exporter pod per SolrCloud managed by a K8s deployment.

For our example `dev-prom-exporter`, the service name is: `dev-prom-exporter-solr-metrics`

Take a quick look at the labels on the service as you'll need them to define a service monitor in the next step.
```bash
kubectl get svc dev-prom-exporter-solr-metrics --show-labels
```

Also notice the ports that are exposed for this service:
```bash
kubectl get svc dev-prom-exporter-solr-metrics --output jsonpath="{@.spec.ports}"
```
You should see output similar to:
```json
[{"name":"solr-metrics","port":80,"protocol":"TCP","targetPort":8080}]
```

### Create a Service Monitor
The Prometheus operator (deployed with the Prometheus stack) uses service monitors to find which services to scrape metrics from. Thus, we need to define a service monitor for our exporter service `dev-prom-exporter-solr-metrics`.
If you're not using the Prometheus operator, then you do not need a service monitor as Prometheus will scrape metrics using the `prometheus.io/*` pod annotations on the exporter service; see [Prometheus Configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/). 

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: solr-metrics
  labels:
    release: mon
spec:
  selector:
    matchLabels:
      solr-prometheus-exporter: dev-prom-exporter
  namespaceSelector:
    matchNames:
    - dev
  endpoints:
  - port: solr-metrics
    interval: 20s
```

There are a few important aspects of this service monitor definition:
* The `release: mon` label associates this service monitor with the Prometheus operator; recall that we used `mon` as the Helm release when installing our Prometheus stack
* The Prometheus operator uses the `solr-prometheus-exporter: dev-prom-exporter` label selector to find the service to scrape metrics from, which of course is the `dev-prom-exporter-solr-metrics` service created by the Solr operator.
* The Prometheus operator uses `dev` to match the namespace (`namespaceSelector.matchNames`) where our SolrCloud and Prometheus exporter services are deployed
* The `endpoints` section identifies the port to scrape metrics from and the scrape interval; recall our service exposes the port as `solr-metrics`  

Save the service monitor YAML to a file, such as `dev-prom-service-monitor.yaml` and apply to the `monitoring` namespace:
```bash
kubectl apply -f dev-prom-service-monitor.yaml -n monitoring
```

Prometheus is now configured to scrape metrics from the exporter service.

### Load Solr Dashboard in Grafana

You can expose Grafana via a LoadBalancer (or Ingress) but for now, we'll just open a port-forward to port 3000 to access Grafana:
```bash
GRAFANA_POD_ID=$(kubectl get pod -l app.kubernetes.io/name=grafana --no-headers -o custom-columns=":metadata.name" -n monitoring)
kubectl port-forward -n monitoring $GRAFANA_POD_ID 3000
```
Open Grafana using `localhost:3000` and login with username `admin` and password `prom-operator`.

Once logged into Grafana, import the Solr dashboard JSON corresponding to the version of Solr you're running.

Solr does not export any useful metrics until you have at least one collection defined.

_Note: Solr 8.8.0 and newer versions include an updated dashboard that provides better metrics for monitoring query performance._ 
