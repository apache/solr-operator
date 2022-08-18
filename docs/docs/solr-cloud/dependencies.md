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

## Dependent Kubernetes Resources

What actually gets created when the Solr Cloud is spun up?

```bash
$ kubectl get all

NAME                                       READY   STATUS             RESTARTS   AGE
pod/example-solrcloud-0                    1/1     Running            7          47h
pod/example-solrcloud-1                    1/1     Running            6          47h
pod/example-solrcloud-2                    1/1     Running            0          47h
pod/example-solrcloud-3                    1/1     Running            6          47h
pod/example-solrcloud-zk-0                 1/1     Running            0          49d
pod/example-solrcloud-zk-1                 1/1     Running            0          49d
pod/example-solrcloud-zk-2                 1/1     Running            0          49d
pod/example-solrcloud-zk-3                 1/1     Running            0          49d
pod/example-solrcloud-zk-4                 1/1     Running            0          49d
pod/solr-operator-8449d4d96f-cmf8p         1/1     Running            0          47h
pod/zk-operator-674676769c-gd4jr           1/1     Running            0          49d

NAME                                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)               AGE
service/example-solrcloud-0                ClusterIP   ##.###.###.##    <none>        80/TCP                47h
service/example-solrcloud-1                ClusterIP   ##.###.##.#      <none>        80/TCP                47h
service/example-solrcloud-2                ClusterIP   ##.###.###.##    <none>        80/TCP                47h
service/example-solrcloud-3                ClusterIP   ##.###.##.###    <none>        80/TCP                47h
service/example-solrcloud-common           ClusterIP   ##.###.###.###   <none>        80/TCP                47h
service/example-solrcloud-headless         ClusterIP   None             <none>        8983/TCP              47h
service/example-solrcloud-zk-client        ClusterIP   ##.###.###.###   <none>        21210/TCP             49d
service/example-solrcloud-zk-headless      ClusterIP   None             <none>        22210/TCP,23210/TCP   49d

NAME                                       READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/solr-operator              1/1     1            1           49d
deployment.apps/zk-operator                1/1     1            1           49d

NAME                                       DESIRED   CURRENT   READY   AGE
replicaset.apps/solr-operator-8449d4d96f   1         1         1       2d1h
replicaset.apps/zk-operator-674676769c     1         1         1       49d

NAME                                       READY   AGE
statefulset.apps/example-solrcloud         4/4     47h
statefulset.apps/example-solrcloud-zk      5/5     49d

NAME                                          HOSTS                                                                                       PORTS   AGE
ingress.extensions/example-solrcloud-common   default-example-solrcloud.test.domain,default-example-solrcloud-0.test.domain + 3 more...   80      2d2h

NAME                                       VERSION   DESIREDNODES   NODES   READYNODES   AGE
solrcloud.solr.apache.org/example       8.1.1     4              4       4            47h
```