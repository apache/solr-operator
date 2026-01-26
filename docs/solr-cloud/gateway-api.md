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

# Gateway API

## Overview

The Solr Operator supports using the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/) for external addressability of SolrClouds.
Gateway API is a vendor-neutral, Kubernetes-native API for managing ingress traffic and is the successor to the Ingress API.

When you configure `spec.solrAddressability.external.method: Gateway`, the Solr Operator creates and manages HTTPRoute resources
that route external traffic to your Solr nodes through an existing Gateway resource in your cluster.

## What Gets Created

When Gateway mode is enabled, the Solr Operator automatically creates the following Kubernetes resources:

### HTTPRoute Resources

The operator creates HTTPRoute resources to route traffic to Solr:

- **Common HTTPRoute**: Routes traffic to the common Solr service (load-balanced across all nodes)
  - Named: `<solrcloud-name>-solrcloud-common`
  - Hostname: `<namespace>-<solrcloud-name>-solrcloud.<domainName>`
  
- **Per-Node HTTPRoutes**: Routes traffic directly to individual Solr nodes (when `hideNodes: false`)
  - Named: `<solrcloud-name>-solrcloud-<node-index>`
  - Hostname: `<namespace>-<solrcloud-name>-solrcloud-<node-index>.<domainName>`

All HTTPRoutes are owned by the SolrCloud resource and will be automatically cleaned up when the SolrCloud is deleted.

### Services

The same services are created as with other external addressability methods:
- Common service (load-balanced)
- Headless service (for internal cluster communication)
- Per-node services (when individual node access is enabled)

## What the Operator Assumes

The Gateway mode assumes the following resources already exist in your cluster:

1. **Gateway API CRDs**: The Gateway API CRDs must be installed in your cluster
   - Minimum version: v1.0.0
   - Required CRDs: `Gateway`, `GatewayClass`, `HTTPRoute`
   
2. **Gateway Resource**: A Gateway resource must already exist and be managed by your platform team
   - The operator only manages HTTPRoute resources, not the Gateway itself
   - The Gateway must be configured with appropriate listeners and TLS termination (if needed)
   
3. **Gateway Controller**: A Gateway controller implementation must be running (e.g., NGINX Gateway Fabric, Istio, Envoy Gateway)

The Solr Operator does **not** create or manage Gateway or GatewayClass resources - these are infrastructure-level resources
typically managed by platform administrators and are specific to the Gateway implementation deployed in your cluster (e.g., NGINX Gateway Fabric, Istio, Envoy Gateway).

## Configuration

Configure Gateway mode in your SolrCloud spec:

```yaml
apiVersion: solr.apache.org/v1beta1
kind: SolrCloud
metadata:
  name: example
  namespace: solr-ns
spec:
  replicas: 3
  solrImage:
    tag: "9.7.0"
  solrAddressability:
    external:
      method: Gateway
      domainName: solr.example.com
      useExternalAddress: true
      gateway:
        # Reference to existing Gateway resource(s)
        parentRefs:
        - name: my-gateway
          namespace: gateway-ns
          sectionName: https  # Optional: specific listener name
        
        # Optional: annotations to add to HTTPRoute resources
        annotations:
          example.com/custom-annotation: "value"
        
        # Optional: labels to add to HTTPRoute resources
        labels:
          app: solr
          environment: production
```

### Configuration Options

- **`parentRefs`** (required): List of Gateway resources to attach HTTPRoutes to
  - `name`: Name of the Gateway resource
  - `namespace`: Namespace of the Gateway (can be different from SolrCloud namespace)
  - `sectionName`: Optional listener name within the Gateway
  
- **`annotations`**: Optional annotations to add to all created HTTPRoute resources

- **`labels`**: Optional labels to add to all created HTTPRoute resources

- **`domainName`**: Base domain for constructing hostnames

- **`useExternalAddress`**: When true, Solr nodes use external addresses for inter-node communication

- **`hideCommon`**: Set to true to skip creating the common HTTPRoute (default: false)

- **`hideNodes`**: Set to true to skip creating per-node HTTPRoutes (default: false)

## Backend TLS Policy

When using TLS-enabled Solr (`spec.solrTLS` is configured), the Solr Operator automatically sets `appProtocol: https` on all Services.
The operator also supports creating `BackendTLSPolicy` resources to configure secure connections between the Gateway and Solr backend services.

### Configuring BackendTLSPolicy

The Solr Operator can automatically create and manage `BackendTLSPolicy` resources (Gateway API v1) when configured in the SolrCloud spec:

```yaml
apiVersion: solr.apache.org/v1beta1
kind: SolrCloud
metadata:
  name: example
  namespace: solr-ns
spec:
  replicas: 3
  solrImage:
    tag: "9.7.0"
  # Enable TLS for Solr
  solrTLS:
    pkcs12Secret:
      name: solr-tls-cert
      key: keystore.p12
  solrAddressability:
    external:
      method: Gateway
      domainName: solr.example.com
      gateway:
        parentRefs:
        - name: my-gateway
          namespace: gateway-ns
        # Configure BackendTLSPolicy for secure backend connections
        backendTLSPolicy:
          # Option 1: Reference CA certificate from a ConfigMap (default)
          caCertificateRefs:
          - name: solr-ca-cert
            # kind: ConfigMap  # Optional, defaults to ConfigMap
            # group: ""        # Optional, defaults to "" (core API)
          
          # Option 2: Use well-known CA certificates
          # wellKnownCACertificates: "System"
```

The generated `BackendTLSPolicy` will look like:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: example-solrcloud-common
  namespace: solr-ns
spec:
  targetRefs:
  - name: example-solrcloud-common
    kind: Service
    group: ""
  validation:
    hostname: example-solrcloud-common
    caCertificateRefs:
    - group: ""
      kind: ConfigMap
      name: solr-ca-cert
```

### BackendTLSPolicy Options

The `backendTLSPolicy` field supports two mutually exclusive options:

- **`caCertificateRefs`**: References to Kubernetes ConfigMaps or Secrets containing CA certificates
  - `name`: Name of the ConfigMap or Secret (required)
  - `kind`: Resource kind (optional, defaults to "ConfigMap")
  - `group`: API group (optional, defaults to "" for core API)
  - Maximum of 8 certificate references
  - **Note**: ConfigMaps must contain the CA certificate in a key named `ca.crt`

- **`wellKnownCACertificates`**: Use system CA certificates (e.g., "System")
  - Only one of `caCertificateRefs` or `wellKnownCACertificates` can be specified

When configured, the operator creates:
- `BackendTLSPolicy` for the common service (if not hidden)
- `BackendTLSPolicy` for each node service (if not hidden)

These policies configure the Gateway to validate backend TLS certificates and establish secure connections to Solr pods.

### Gateway Implementation Support

**Note**: `BackendTLSPolicy` is part of the Gateway API standard (v1alpha2+), but support varies by implementation:

| Gateway Implementation | BackendTLSPolicy Support |
|------------------------|--------------------------|
| **Standard Gateway API** | ✅ v1alpha2+ |
| **Envoy Gateway** | ✅ Full support |
| **Istio** | ⚠️ Use `DestinationRule` instead |
| **NGINX Gateway Fabric** | ✅ Experimental support |
| **GKE Gateway** | ⚠️ Automatic via `appProtocol` |

Refer to your Gateway implementation's documentation for specific backend TLS configuration requirements.

## Complete Example

```yaml
apiVersion: solr.apache.org/v1beta1
kind: SolrCloud
metadata:
  name: example
  namespace: solr-ns
spec:
  replicas: 3
  solrImage:
    tag: "9.7.0"
  solrAddressability:
    external:
      method: Gateway
      domainName: solr.example.com
      useExternalAddress: true
      gateway:
        parentRefs:
        - name: my-gateway
          namespace: gateway-ns
        annotations:
          example.com/rate-limit: "1000"
        labels:
          app: solr
```

This configuration will create:
- HTTPRoute: `example-solrcloud-common` → `solr-ns-example-solrcloud.solr.example.com`
- HTTPRoute: `example-solrcloud-0` → `solr-ns-example-solrcloud-0.solr.example.com`
- HTTPRoute: `example-solrcloud-1` → `solr-ns-example-solrcloud-1.solr.example.com`
- HTTPRoute: `example-solrcloud-2` → `solr-ns-example-solrcloud-2.solr.example.com`

## References

- [Gateway API Documentation](https://gateway-api.sigs.k8s.io/)
- [BackendTLSPolicy (GEP-1897)](https://gateway-api.sigs.k8s.io/geps/gep-1897/)
- [Kubernetes Service appProtocol](https://kubernetes.io/docs/concepts/services-networking/service/#application-protocol)
