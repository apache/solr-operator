# Running the Solr Operator

## Using the Solr Operator Helm Chart

The easiest way to run the Solr Operator is via the [provided Helm Chart](/helm/solr-operator).

The helm chart provides abstractions over the Input Arguments described below, and should work with any official images in docker hub.

### How to install via Helm

Currently you need a local version of the Helm chart downloaded before you can install the operator.

```bash
# Download the operator
OPERATOR_VER=0.2.5
curl https://codeload.github.com/bloomberg/solr-operator/tar.gz/v$OPERATOR_VER | tar xz
ln -s -f solr-operator-$OPERATOR_VER solr-operator

# Install the Zookeeper operator (and etcd operator even if we don't use it)
kubectl apply -f solr-operator/example/dependencies/

# Install the operator (specifying ingressBaseDomain to match our ingressController)
helm install --set-string ingressBaseDomain=ing.local.domain \
    solr-operator solr-operator/helm/solr-operator
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

Two Docker images are published to [DockerHub](https://hub.docker.com/r/bloomberg/solr-operator), both based off of the same base image.

- [Builder Image](build/Dockerfile.build) - Downloads gomod dependencies, builds operator executable (This is not published, only used to build the following images)
- [Slim Image](build/Dockerfile.slim) - Contains only the operator executable, with the operator as the entry point
- [Vendor Image](build/Dockerfile.slim) - Contains the operator executable as well as all dependencies (at `/solr-operator-vendor-sources`)

In order to run the Solr Operator, you will only need the Slim Image.

## Solr Operator Input Args

* **-zookeeper-operator** Whether or not to use the Zookeeper Operator to create dependent Zookeeepers.
                          Required to use the `ProvidedZookeeper.Zookeeper` option within the Spec.
                          If _true_, then a Zookeeper Operator must be running for the cluster.
                          ( _true_ | _false_ , defaults to _false_)
* **-etcd-operator** Whether or not to use the Etcd Operator to create dependent Zetcd clusters.
                     Required to use the `ProvidedZookeeper.Zetcd` option within the Spec.
                     If _true_, then an Etcd Operator must be running for the cluster.
                     ( _true_ | _false_ , defaults to _false_)
* **-ingress-base-domain** If you desire to make solr externally addressable via ingresses, a base ingress domain is required.
                        Solr Clouds will be created with ingress rules at `*.(ingress-base-domain)`.
                        ( _optional_ , e.g. `ing.base.domain` )
                        
    