# Gateway API with TLS-Enabled Solr

## Overview

When `spec.solrTLS` is enabled, the Solr Operator automatically sets `appProtocol: https` on all Services and creates HTTPRoute resources. **Backend TLS policy configuration is the user's responsibility** and varies by Gateway implementation to maintain vendor neutrality.

## Operator vs User Responsibilities

**Operator automatically handles:**
- Sets `appProtocol: https` on Services (common, headless, per-node)
- Creates HTTPRoute resources

**Users must configure:**

| Gateway Implementation | Backend TLS Method |
|------------------------|--------------------|
| **Standard Gateway API** | `BackendTLSPolicy` (requires proper SANs) |
| **Istio** | `DestinationRule` |
| **GKE Gateway** | Automatic (via `appProtocol`) |
| **kGateway** | `BackendConfigPolicy` (flexible CA validation) |
| **NGINX Gateway Fabric** | Annotations or `BackendTLSPolicy` |

## Configuration by Gateway Implementation

Refer to your Gateway implementation's documentation for configuring backend TLS policies. Common approaches include:

- **Standard Gateway API**: Use `BackendTLSPolicy` (v1alpha3+)
- **Istio**: Use `DestinationRule` with TLS configuration
- **GKE Gateway**: Automatic via `appProtocol: https`
- **Implementation-specific**: Consult your Gateway provider's documentation for backend TLS configuration

## Complete Example

```yaml
# 1. SolrCloud with TLS and Gateway API
apiVersion: solr.apache.org/v1beta1
kind: SolrCloud
metadata:
  name: example
  namespace: solr-ns
spec:
  replicas: 3
  solrImage:
    tag: "9.7.0"
  solrTLS:
    pkcs12Secret:
      name: solr-tls-cert
      key: keystore.p12
    keystorePasswordSecret:
      name: solr-tls-cert
      key: keystore-password
  solrAddressability:
    external:
      method: Gateway
      domainName: solr.example.com
      gateway:
        parentRefs:
        - name: solr-gateway
          namespace: gateway-ns
          sectionName: https

---
# 2. Gateway with HTTPS listener
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: solr-gateway
  namespace: gateway-ns
spec:
  gatewayClassName: <your-gateway-class>
  listeners:
  - name: https
    protocol: HTTPS
    port: 443
    tls:
      mode: Terminate
      certificateRefs:
      - name: external-tls-cert
    allowedRoutes:
      namespaces:
        from: All

---
# 3. Backend TLS Policy (choose based on your Gateway - see examples above)
```

## Why This Approach?

**Ingress** automatically adds NGINX-specific annotations (`nginx.ingress.kubernetes.io/backend-protocol: HTTPS`).

**Gateway API** uses vendor-neutral `appProtocol: https` and lets users configure backend TLS policies for their specific Gateway implementation.

**Benefits:**
- Maintains portability across Gateway implementations
- Follows Kubernetes standards
- Avoids vendor lock-in

## References

- [Gateway API Documentation](https://gateway-api.sigs.k8s.io/)
- [BackendTLSPolicy (GEP-1897)](https://gateway-api.sigs.k8s.io/geps/gep-1897/)
- [Kubernetes Service appProtocol](https://kubernetes.io/docs/concepts/services-networking/service/#application-protocol)
