# Developing the Solr Operator

This tutorial shows how to setup Solr under Kubernetes on your local mac. The plan is as follows:

 1. [Setup Docker for Mac with K8S](#setup-docker-for-mac-with-k8s)
 2. [Install an Ingress Controller to reach the cluster on localhost](#install-an-ingress-controller)
 3. [Install Solr Operator](#install-solr-operator)
 4. [Start your Solr cluster](#start-your-solr-cluster)
 5. [Create a collection and index some documents](#create-a-collection-and-index-some-documents)
 6. [Scale from 3 to 5 nodes](#scale-from-3-to-5-nodes)
 7. [Upgrade to newer Solr version](#upgrade-to-newer-version)
 8. [Install Kubernetes Dashboard (optional)](#install-kubernetes-dashboard-optional)
 9. [Delete the solrCloud cluster named 'example'](#delete-the-solrcloud-cluster-named-example)

### Setup Docker for Mac with K8S with an Ingress Controller

Please follow the instructions from the [local tutorial](local_tutorial.md#setup-docker-for-mac-with-k8s).

### Installing the necessary dependencies

Install the Zookeeper & Etcd Operators, which this operator depends on by default.
Each is optional, as described in the [Zookeeper](#zookeeper-reference) section.

```bash
$ kubectl apply -f example/dependencies
```

Install necessary dependencies for building and deploying the operator.
```bash
$ export PATH="$PATH:$GOPATH/bin" # You likely want to add this line to your ~/.bashrc or ~/.bash_aliases
$ ./hack/install_dependencies.sh
```

### Building the Solr CRDs

If you have changed anything in the [APIs directory](/api/v1beta1), you will need to run the following command to regenerate all Solr CRDs.

```bash
$ make manifests
```

In order to apply these CRDs, merely run the following:

```bash
$ make install
```

### Building and Running the Solr Operator

Building

You can also deploy the Solr Operator by using our provided [Helm Chart](/helm/solr-operator/README.md).

#### Building the Docker image

Building and releasing a test operator image with a custom Docker namespace.

```bash
$ NAMESPACE=your-namespace/ make docker-build docker-push
```

You can test the vendor docker container by running

```bash
$ NAMESPACE=your-namespace/ make docker-vendor-build docker-vendor-push
```

### Before you create a PR

The CRD should be updated anytime you update the API.

```bash
$ make manifests
```

Beware that you must be running an updated version of `controller-gen`. To update to a compatible version, run:

```bash
$ go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.2
```

Make sure that you have updated the go.mod file:

```bash
$ make mod-tidy
```