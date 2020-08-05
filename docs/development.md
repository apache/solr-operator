# Developing the Solr Operator

This page details the steps for developing the Solr Operator, and all necessary steps to follow before creating a PR to the repo.

 - [Setup](#setup)
    - [Setup Docker for Mac with K8S](#setup-docker-for-mac-with-k8s-with-an-ingress-controller)
    - [Install the necessary Dependencies](#install-the-necessary-dependencies)
 - [Build the Solr CRDs](#build-the-solr-crds)
 - [Build and Run the Solr Operator](#build-and-run-local-versions)
    - [Build the Solr Operator](#building-the-solr-operator)
    - [Running the Solr Operator](#running-the-solr-operator)
 - [Steps to take before creating a PR](#before-you-create-a-pr)
 
## Setup

### Setup Docker for Mac with K8S with an Ingress Controller

Please follow the instructions from the [local tutorial](local_tutorial.md#setup-docker-for-mac-with-k8s).

### Install the necessary dependencies

Install the Zookeeper, which this operator depends on by default.
Each is optional, as described in the [Zookeeper Reference](solr-cloud/solr-cloud-crd.md#zookeeper-reference) section in the CRD docs.

```bash
$ kubectl apply -f example/dependencies
```

Install necessary dependencies for building and deploying the operator.
```bash
$ export PATH="$PATH:$GOPATH/bin" # You likely want to add this line to your ~/.bashrc or ~/.bash_aliases
$ ./hack/install_dependencies.sh
```

Beware that you must be running an updated version of `controller-gen`. To update to a compatible version, run:

```bash
$ go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.2
```

## Build the Solr CRDs

If you have changed anything in the [APIs directory](/api/v1beta1), you will need to run the following command to regenerate all Solr CRDs.

```bash
$ make manifests
```

In order to apply these CRDs to your kube cluster, merely run the following:

```bash
$ make install
```

## Build and Run local versions

It is very useful to build and run your local version of the operator to test functionality.

### Building the Solr Operator

#### Building a Go binary

Building the Go binary files is quite straightforward:

```bash
$ go build
```

This is useful for testing that your code builds correctly, as well as using the `make run` command detailed below.

#### Building the Docker image

Building and releasing a test operator image with a custom Docker namespace.

```bash
$ NAMESPACE=your-namespace/ make docker-build docker-push
```

You can test the vendor docker container by running

```bash
$ NAMESPACE=your-namespace/ make docker-vendor-build docker-vendor-push
```

You can control the namespace and version for your solr-operator docker image via the ENV variables:
- `NAMESPACE`, defaults to `bloomberg/`. **This must end with a forward slash.** This can also include the docker repository information for private repos.
- `NAME`, defaults to `solr-operator`.
- `VERSION`, defaults to the git HEAD tag. (e.g. `v0.2.5-1-g06f4e2a`).  
You can check what version you are using by running `make version`.

The image will be created under the tag `$(NAMESPACE)$(NAME):$(VERSION)` as well as `$(NAMESPACE)$(NAME):latest`.


### Running the Solr Operator

There are a few options for running the Solr Operator version you are developing.

- You can deploy the Solr Operator by using our provided [Helm Chart](/helm/solr-operator/README.md).
You will need to [build a docker image](#building-the-docker-image) for your version of the operator.
Then update the values for the helm chart to use the version that you have built.
- There are two useful `make` commands provided to help with running development versions of the operator:
    - `make run` - This command will start the solr-operator process locally (not within kubernetes).
    This does not require building a docker image.
    - `make deploy` - This command will apply the docker image with your local version to your kubernetes cluster.
    This requires [building a docker image](#building-the-docker-image).
    
**Warning**: If you are running kubernetes locally and do not want to push your image to docker hub or a private repository, you will need to set the `imagePullPolicy: Never` on your Solr Operator Deployment.
That way Kubernetes does not try to pull your image from whatever repo it is listed under (or docker hub by default).

## Testing

If you are creating new functionality for the operator, please include that functionality in an existing test or a new test before creating a PR.
Most tests can be found in the [controller](/controllers) directory, with names that end in `_test.go`.

PRs will automatically run the unit tests, and will block merging if the tests fail.

You can run these tests locally via the following make command:

```bash
$ make test
```

## Before you create a PR

The CRD should be updated anytime you update the API.

```bash
$ make manifests
```


Make sure that you have updated the go.mod file:

```bash
$ make mod-tidy
```

Finally, as mentioned on the Repo's README, you will need to sign all commits included in the PR.
More information can be found on the [organization's contibutions documentation](https://github.com/bloomberg/.github/blob/master/CONTRIBUTING.md#contribution-licensing).