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