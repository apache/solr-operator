# Running the Solr Operator

## Using the Solr Operator Helm Chart

The easiest way to run the Solr Operator is via the [provided Helm Chart](https://artifacthub.io/packages/helm/apache-solr/solr-operator).

The helm chart provides abstractions over the Input Arguments described below, and should work with any official images in docker hub.

### How to install via Helm

The official documentation for installing the Solr Operator Helm chart can be found on [Artifact Hub](https://artifacthub.io/packages/helm/apache-solr/solr-operator).

The first step is to add the Solr Operator helm repository.

```bash
$ helm repo add apache-solr https://apache.github.io/lucene-solr-operator/charts
$ helm repo update
```


Next, install the Solr Operator chart. Note this is using Helm v3, use the official Helm chart documentation linked to above.
This will install the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator) by default.

```bash
$ helm install solr-operator apache-solr/solr-operator
```

After installing, you can check to see what lives in the cluster to make sure that the Solr and ZooKeeper operators have started correctly.
```bash
$ kubectl get all

NAME                                              READY   STATUS             RESTARTS   AGE
pod/solr-operator-8449d4d96f-cmf8p                1/1     Running            0          47h
pod/zookeeper-operator-674676769c-gd4jr           1/1     Running            0          49d

NAME                                              READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/solr-operator                     1/1     1            1           49d
deployment.apps/zookeeper-operator                1/1     1            1           49d

NAME                                              DESIRED   CURRENT   READY   AGE
replicaset.apps/solr-operator-8449d4d96f          1         1         1       2d1h
replicaset.apps/zookeeper-operator-674676769c     1         1         1       49d
```

After inspecting the status of you Kube cluster, you should see a deployment for the Solr Operator as well as the Zookeeper Operator.

## Solr Operator Docker Images

Two Docker images are published to [DockerHub](https://hub.docker.com/r/apache/solr-operator), both based off of the same base image.

- [Builder Image](build/Dockerfile.build) - Downloads gomod dependencies, builds operator executable (This is not published, only used to build the following images)
- [Slim Image](build/Dockerfile.slim) - Contains only the operator executable, with the operator as the entry point
- [Vendor Image](build/Dockerfile.slim) - Contains the operator executable as well as all dependencies (at `/solr-operator-vendor-sources`)

In order to run the Solr Operator, you will only need the Slim Image.

## Solr Operator Input Args

* **-zookeeper-operator** Whether or not to use the Zookeeper Operator to create dependency Zookeeepers.
                          Required to use the `spec.zookeeperRef.provided` option.
                          If _true_, then a Zookeeper Operator must be running for the cluster.
                          (_true_ | _false_ , defaults to _false_)
                        
    