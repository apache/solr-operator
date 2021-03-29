# Solr on Kubernetes on local Mac

This tutorial shows how to setup Solr under Kubernetes on your local mac. The plan is as follows:

 1. [Setup Kubernetes and Dependencies](#setup-kubernetes-and-dependencies)
    1. [Setup Docker for Mac with K8S](#setup-docker-for-mac-with-k8s)
    2. [Install an Ingress Controller to reach the cluster on localhost](#install-an-ingress-controller)
 3. [Install Solr Operator](#install-the-solr-operator)
 4. [Start your Solr cluster](#start-an-example-solr-cloud-cluster)
 5. [Create a collection and index some documents](#create-a-collection-and-index-some-documents)
 6. [Scale from 3 to 5 nodes](#scale-from-3-to-5-nodes)
 7. [Upgrade to newer Solr version](#upgrade-to-newer-version)
 8. [Install Kubernetes Dashboard (optional)](#install-kubernetes-dashboard-optional)
 9. [Delete the solrCloud cluster named 'example'](#delete-the-solrcloud-cluster-named-example)

## Setup Kubernetes and Dependencies

### Setup Docker for Mac with K8s

```bash
# Install Homebrew, if you don't have it already
/bin/bash -c "$(curl -fsSL \
	https://raw.githubusercontent.com/Homebrew/install/master/install.sh)"

# Install Docker Desktop for Mac (use edge version to get latest k8s)
brew cask install docker-edge

# Enable Kubernetes in Docker Settings, or run the command below:
sed -i -e 's/"kubernetesEnabled" : false/"kubernetesEnabled" : true/g' \
    ~/Library/Group\ Containers/group.com.docker/settings.json

# Start Docker for mac from Finder, or run the command below
open /Applications/Docker.app

# Install Helm, which we'll use to install the operator, and 'watch'
brew install helm watch
```

### Install an Ingress Controller

Kubernetes services are by default only accessible from within the k8s cluster. To make them adressable from our laptop, we'll add an ingress controller

```bash
# Install the nginx ingress controller
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/cloud/deploy.yaml

# Inspect that the ingress controller is running by visiting the Kubernetes dashboard 
# and selecting namespace `ingress-nginx`, or running this command:
kubectl get all --namespace ingress-nginx

# Edit your /etc/hosts file (`sudo vi /etc/hosts`) and replace the 127.0.0.1 line with:
127.0.0.1	localhost default-example-solrcloud.ing.local.domain ing.local.domain default-example-solrcloud-0.ing.local.domain default-example-solrcloud-1.ing.local.domain default-example-solrcloud-2.ing.local.domain dinghy-ping.localhost
```

Once we have installed Solr to our k8s, this will allow us to address the nodes locally.

## Install the Solr Operator

Now that we have the prerequisites setup, let us install Solr Operator which will let us easily manage a large Solr cluster:

Before installing the Solr Operator, we need to install the [Zookeeper Operator](https://github.com/pravega/zookeeper-operator).
Eventually this will be a dependency on the helm chart, but for now we can run an easy `kubectl apply`.

```bash
kubectl apply -f https://apache.github.io/solr-operator/example/dependencies/zk_operator.yaml
```

Now add the Solr Operator Helm repository. (You should only need to do this once)

```bash
$ helm repo add solr-operator https://apache.github.io/lucene-solr-operator/charts
```

Next, install the Solr Operator chart. Note this is using Helm v3, in order to use Helm v2 please consult the [Helm Chart documentation](https://hub.helm.sh/charts/solr-operator/solr-operator).

```bash
# Install the operator (specifying ingressBaseDomain to match our ingressController)
$ helm install solr-operator solr-operator/solr-operator --set-string ingressBaseDomain=ing.local.domain
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

## Start an example Solr Cloud cluster

To start a Solr Cloud cluster, we will create a yaml that will tell the Solr Operator what version of Solr Cloud to run, and how many nodes, with how much memory etc.

```bash
# Create a spec for a 3-node cluster v8.3 with 300m RAM each:
cat <<EOF > solrCloud-example.yaml
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCloud
metadata:
  name: example
spec:
  replicas: 3
  solrImage:
    tag: "8.3"
  solrJavaMem: "-Xms300m -Xmx300m"
EOF

# Install Solr from that spec
kubectl apply -f solrCloud-example.yaml

# The solr-operator has created a new resource type 'solrclouds' which we can query
# Check the status live as the deploy happens
kubectl get solrclouds -w

# Open a web browser to see a solr node:
# Note that this is the service level, so will round-robin between the nodes
open "http://default-example-solrcloud.ing.local.domain/solr/#/~cloud?view=nodes"
```

## Create a collection and index some documents

We'll use the Operator's built in collection creation option

```bash
# Create the spec
cat <<EOF > collection.yaml
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCollection
metadata:
  name: mycoll
spec:
  solrCloud: example
  collection: mycoll
  autoAddReplicas: true
  routerName: compositeId
  numShards: 1
  replicationFactor: 3
  maxShardsPerNode: 2
  collectionConfigName: "_default"
EOF

# Execute the command and check in Admin UI that it succeeds
kubectl apply -f collection.yaml

# Check in Admin UI that collection is created
open "http://default-example-solrcloud.ing.local.domain/solr/#/~cloud?view=graph"
```

Now index some documents into the empty collection.
```bash
curl -XPOST -H "Content-Type: application/json" \
    -d '[{id: 1}, {id: 2}, {id: 3}, {id: 4}, {id: 5}, {id: 6}, {id: 7}, {id: 8}]' \
    "http://default-example-solrcloud.ing.local.domain/solr/mycoll/update/"
```

## Scale from 3 to 5 nodes

So we wish to add more capacity. Scaling the cluster is a breeze.

```
# Issue the scale command
kubectl scale --replicas=5 solrcloud/example
```

After issuing the scale command, start hitting the "Refresh" button in the Admin UI.
You will see how the new Solr nodes are added.
You can also watch the status via the `kubectl get solrclouds` command:

```bash
kubectl get solrclouds -w

# Hit Control-C when done
```

## Upgrade to newer version

So we wish to upgrade to a newer Solr version:

```
# Take note of the current version, which is 8.3.1
curl -s http://default-example-solrcloud.ing.local.domain/solr/admin/info/system | grep solr-i

# Update the solrCloud configuratin with the new version, keeping 5 nodes
cat <<EOF > solrCloud-example.yaml
apiVersion: solr.bloomberg.com/v1beta1
kind: SolrCloud
metadata:
  name: example
spec:
  replicas: 5
  solrImage:
    tag: "8.7"
  solrJavaMem: "-Xms300m -Xmx300m"
EOF

# Apply the new config
# Click the 'Show all details" button in Admin UI and start hitting the "Refresh" button
# See how the operator upgrades one pod at a time. Solr version is in the 'node' column
# You can also watch the status with the 'kubectl get solrclouds' command
kubectl apply -f solrCloud-example.yaml
kubectl get solrclouds -w

# Hit Control-C when done
```

## Install Kubernetes Dashboard (optional)

Kubernetes Dashboard is a web interface that gives a better overview of your k8s cluster than only running command-line commands. This step is optional, you don't need it if you're comfortable with the cli.

```
# Install the Dashboard
kubectl apply -f https://raw.githubusercontent.com/kubernetes/dashboard/v2.0.4/aio/deploy/recommended.yaml

# You need to authenticate with the dashboard. Get a token:
kubectl -n kubernetes-dashboard describe secret \
    $(kubectl -n kubernetes-dashboard get secret | grep default-token | awk '{print $1}') \
    | grep "token:" | awk '{print $2}'

# Start a kube-proxy in the background (it will listein on localhost:8001)
kubectl proxy &

# Open a browser to the dashboard (note, this is one long URL)
open "http://localhost:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/#/overview?namespace=default"

# Select 'Token' in the UI and paste the token from last step (starting with 'ey...')
```

## Delete the solrCloud cluster named 'example'

```
kubectl delete solrcloud example
```
