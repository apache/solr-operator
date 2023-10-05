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

# Known Issues

## Solr Cloud

- You may be seeing timeouts in your Solr logs after a rolling update.
  This can be caused by DNS caching in CoreDNS (the default DNS for Kubernetes).
  This can be fixed by reducing the kubernetes cache TTL to 5-10 seconds.
  Please refer to this ticket for more information: https://github.com/kubernetes/kubernetes/issues/92559
  \
  Fix: In the `kube-system` namespace, there will be a `ConfigMap` with name `coredns`. It will contain a section:
  ```
  kubernetes cluster.local in-addr.arpa ip6.arpa {
   ...
   ttl 30
  ...
  }
  ```
  Edit the `ttl` value to be `5`, CoreDNS will automatically see a change in the configMap and reload it.