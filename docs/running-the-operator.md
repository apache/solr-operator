# Running the Solr Operator

## Using the Solr Operator Helm Chart

The easiest way to run the Solr Operator is via the [provided Helm Chart](https://artifacthub.io/packages/helm/apache-solr/solr-operator).

The helm chart provides abstractions over the Input Arguments described below, and should work with any official images in docker hub.

### How to install via Helm

The official documentation for installing the Solr Operator Helm chart can be found on [Artifact Hub](https://artifacthub.io/packages/helm/apache-solr/solr-operator).

The first step is to add the Solr Operator helm repository.

```bash
$ helm repo add apache-solr https://solr.apache.org/charts
$ helm repo update
```

Next, install the Solr Operator chart. Note this is using Helm v3, use the official Helm chart documentation linked to above.
This will install the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator) by default.

```bash
$ kubectl create -f https://solr.apache.org/operator/downloads/crds/v0.3.0/all-with-dependencies.yaml
$ helm install solr-operator apache-solr/solr-operator --version 0.3.0
```

_Note that the Helm chart version does not contain a `v` prefix, which the downloads version does. The Helm chart version is the only part of the Solr Operator release that does not use the `v` prefix._


After installing, you can check to see what lives in the cluster to make sure that the Solr and ZooKeeper operators have started correctly.
```bash
$ kubectl get all

NAME                                                   READY   STATUS             RESTARTS   AGE
pod/solr-operator-8449d4d96f-cmf8p                     1/1     Running            0          47h
pod/solr-operator-zookeeper-operator-674676769c-gd4jr  1/1     Running            0          49d

NAME                                              READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/solr-operator                     1/1     1            1           49d
deployment.apps/solr-operator-zookeeper-operator  1/1     1            1           49d

NAME                                                         DESIRED   CURRENT   READY   AGE
replicaset.apps/solr-operator-8449d4d96f                     1         1         1       2d1h
replicaset.apps/solr-operator-zookeeper-operator-674676769c  1         1         1       49d
```

After inspecting the status of you Kube cluster, you should see a deployment for the Solr Operator as well as the Zookeeper Operator.

### Managing CRDs

Helm 3 automatically installs the Solr CRDs in the /crds directory, so no further action is needed when first installing the Operator.

If you have solr operator installations in multiple namespaces that are managed separately, you will likely want to skip installing CRDs when installing the chart.
This can be done with the `--skip-crds` helm option.

```bash
helm install solr-operator apache-solr/solr-operator --skip-crds --namespace solr
```

**Helm will not upgrade CRDs once they have been installed.
Therefore, if you are upgrading from a previous Solr Operator version, be sure to install the most recent CRDs first.**

You can update the released Solr CRDs with the following URL:
```bash
kubectl replace -f "https://solr.apache.org/operator/downloads/crds/<version>/<name>.yaml"
```

Examples:
- `https://solr.apache.org/operator/downloads/crds/v0.3.0/all.yaml`  
  Includes all Solr CRDs in the `v0.3.0` release
- `https://solr.apache.org/operator/downloads/crds/v0.2.7/all-with-dependencies.yaml`  
  Includes all Solr CRDs and dependency CRDs in the `v0.2.7` release
- `https://solr.apache.org/operator/downloads/crds/v0.2.8/solrclouds.yaml`  
  Just the SolrCloud CRD in the `v0.2.8` release

#### The ZookeeperCluster CRD

If you use the provided Zookeeper Cluster in the SolrCloud Spec, it is important to make sure you have the correct `ZookeeperCluster` CRD installed as well.

The Zookeeper Operator Helm chart includes its CRDs when installing, however the way the CRDs are managed can be considered risky.
If we let the Zookeeper Operator Helm chart manage the Zookeeper CRDs, then users could see outages when [uninstalling the chart](#uninstalling-the-chart).
Therefore, by default, we tell the Zookeeper Operator to not install the Zookeeper CRDs.
You can override this, assuming the risks, by setting `zookeeper-operator.crd.create: true`.

For manual installation of the ZookeeperCluster CRD, you can find the file in the [zookeeper-operator repo](https://github.com/pravega/zookeeper-operator/blob/master/deploy/crds/zookeeper.pravega.io_zookeeperclusters_crd.yaml), for the correct release,
or use the convenience download locations provided below.
The Solr CRD releases have bundled the ZookeeperCluster CRD required in each version.

```bash
# Install all Solr CRDs as well as the dependency CRDS (ZookeeperCluster) for the given version of the Solr Operator
kubectl create -f "https://solr.apache.org/operator/downloads/crds/<solr operator version>/all-with-dependencies.yaml"

# Install just the ZookeeperCluster CRD used in the given version of the Solr Operator
kubectl create -f "https://solr.apache.org/operator/downloads/crds/<solr operator version>/zookeeperclusters.yaml"
```

Examples:
- `https://solr.apache.org/operator/downloads/crds/v0.3.0/all-with-dependencies.yaml`  
  Includes all Solr CRDs and dependency CRDs, including `ZookeeperCluster`, in the `v0.3.0` Solr Operator release
- `https://solr.apache.org/operator/downloads/crds/v0.2.8/zookeeperclusters.yaml`  
  Just the ZookeeperCluster CRD required in the `v0.2.8` Solr Operator release

## Solr Operator Docker Images

The Solr Operator Docker image is published to Dockerhub at [apache/solr-operator](https://hub.docker.com/r/apache/solr-operator).
The [Dockerfile](/build/Dockerfile) builds from scratch source, and copies all necessary information to a very limited image.
The final image will only contain the solr-operator binary and necessary License information.

## Solr Operator Input Args

* **-zookeeper-operator** Whether or not to use the Zookeeper Operator to create dependency Zookeeepers.
                          Required to use the `spec.zookeeperRef.provided` option.
                          If _true_, then a Zookeeper Operator must be running for the cluster.
                          (_true_ | _false_ , defaults to _false_)
                        
    