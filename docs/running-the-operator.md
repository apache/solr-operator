# Running the Solr Operator

### Installing the Zookeeper Operator

Before installing the Solr Operator, we need to install the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator).
This is because the Solr Operator, in most instances, relies on the Zookeeper Operator to create the Zookeeper clusters that Solr coordinates through.
Eventually this will be a dependency on the helm chart, but for now we can run an easy `kubectl apply`.

```bash
kubectl apply -f https://apache.github.io/lucene-solr-operator/example/dependencies/zk_operator.yaml
```

## Using the Solr Operator Helm Chart

The easiest way to run the Solr Operator is via the [provided Helm Chart](https://hub.helm.sh/charts/solr-operator/solr-operator).

The helm chart provides abstractions over the Input Arguments described below, and should work with any official images in docker hub.

### How to install via Helm

The official documenation for installing the Solr Operator Helm chart can be found on [Artifact Hub](https://artifacthub.io/packages/helm/apache-solr/solr-operator).

The first step is to add the Solr Operator helm repository.

```bash
$ helm repo add apache-solr https://apache.github.io/lucene-solr-operator/charts
```


Next, install the Solr Operator chart. Note this is using Helm v3, use the official Helm chart documentation linked to above.

```bash
$ helm install solr-operator apache-solr/solr-operator
```

After installing, you can check to see what lives in the cluster to make sure that the Solr and ZooKeeper operators have started correctly.
```
$ kubectl get all

NAME                                       READY   STATUS             RESTARTS   AGE
pod/solr-operator-8449d4d96f-cmf8p         1/1     Running            0          47h
pod/zk-operator-674676769c-gd4jr           1/1     Running            0          49d

NAME                                       READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/solr-operator              1/1     1            1           49d
deployment.apps/zk-operator                1/1     1            1           49d

NAME                                       DESIRED   CURRENT   READY   AGE
replicaset.apps/solr-operator-8449d4d96f   1         1         1       2d1h
replicaset.apps/zk-operator-674676769c     1         1         1       49d
```

After inspecting the status of you Kube cluster, you should see a deployment for the Solr Operator as well as the Zookeeper Operator.

## Solr Operator Docker Images

The Solr Operator Docker image is published to Dockerhub at [apache/solr-operator](https://hub.docker.com/r/apache/solr-operator).
The [Dockerfile](/build/Dockerfile) builds from scratch source, and copies all necessary information to a very limited image.
The final image will only contain the solr-operator binary and necessary License information.

## Solr Operator Input Args

* **-zookeeper-operator** Whether or not to use the Zookeeper Operator to create dependent Zookeeepers.
                          Required to use the `ProvidedZookeeper.Zookeeper` option within the Spec.
                          If _true_, then a Zookeeper Operator must be running for the cluster.
                          ( _true_ | _false_ , defaults to _false_)
                        
    