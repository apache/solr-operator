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

# Solr Clouds

The Solr Operator supports creating and managing Solr Clouds.

To find how to configure the SolrCloud best for your use case, please refer to the [documentation on available SolrCloud CRD options](solr-cloud-crd.md).

This page outlines how to create, update and delete a SolrCloud in Kubernetes.

- [Creation](#creating-an-example-solrcloud)
- [Scaling](#scaling-a-solrcloud)
- [Deletion](#deleting-the-example-solrcloud)
- [Solr Images](#solr-images)
    - [Official Images](#official-solr-images)
    - [Custom Images](#build-your-own-private-solr-images)

## Creating an example SolrCloud

Make sure that the Solr Operator and a Zookeeper Operator are running.

Create an example Solr cloud, with the following configuration.

```bash
$ cat example/test_solrcloud.yaml

apiVersion: solr.apache.org/v1beta1
kind: SolrCloud
metadata:
  name: example
spec:
  replicas: 4
  solrImage:
    tag: 8.1.1
```

Apply it to your Kubernetes cluster.

```bash
$ kubectl apply -f example/test_solrcloud.yaml
$ kubectl get solrclouds

NAME      VERSION   DESIREDNODES   NODES   READYNODES   AGE
example   8.1.1     4              2       1            2m

$ kubectl get solrclouds

NAME      VERSION   DESIREDNODES   NODES   READYNODES   AGE
example   8.1.1     4              4       4            8m
```

What actually gets created when you start a Solr Cloud though?
Refer to the [dependencies outline](dependencies.md) to see what dependent Kuberenetes resources are created in order to run a Solr Cloud.

## Scaling a SolrCloud

The SolrCloud CRD support the Kubernetes `scale` operation, to increase and decrease the number of Solr Nodes that are running within the cloud.

```
# Issue the scale command
kubectl scale --replicas=5 solrcloud/example
```

After issuing the scale command, start hitting the "Refresh" button in the Admin UI.
You will see how the new Solr nodes are added.
You can also watch the status via the `kubectl get solrclouds` command:

```bash
watch -dc kubectl get solrclouds

# Hit Control-C when done
```

### Deleting the example SolrCloud

Delete the example SolrCloud

```bash
$ kubectl delete solrcloud example
```
  
## Solr Images

### Official Solr Images

The Solr Operator is only guaranteed to work with [official Solr images](https://hub.docker.com/_/solr).
However, as long as your custom image is built to be compatible with the official image, things should go smoothly.
This is especially true starting with Solr 9, where the docker image creation is bundled within Solr.
Run `./gradlew docker` in the Solr repository, and your custom Solr additions will be packaged into an officially compliant Solr Docker image.

Please refer to the [Version Compatibility Matrix](../upgrade-notes.md#solr-versions) for more information on what Solr Versions are compatible with the Solr Operator.

Also note that certain features available within the Solr Operator are only supported in newer Solr Versions.
The version compatibility matrix shows the minimum Solr version supported for **most** options.
Please refer to the Solr Reference guide to see what features are enabled for the Solr version you are running.

### Build Your Own Private Solr Images

The Solr Operator supports private Docker repo access for Solr images you may want to store in a private Docker repo. It is recommended to source your image from the official Solr images. 

Using a private image requires you have a K8s secret preconfigured with appropriate access to the image. (type: kubernetes.io/dockerconfigjson)

```
apiVersion: solr.apache.org/v1beta1
kind: SolrCloud
metadata:
  name: example-private-repo-solr-image
spec:
  replicas: 3
  solrImage:
    repository: myprivate-repo.jfrog.io/solr
    tag: 8.2.0
    imagePullSecret: "k8s-docker-registry-secret"
```