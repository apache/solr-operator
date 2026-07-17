<!--
  Licensed to the Apache Software Foundation (ASF) under one or more
  contributor license agreements.  See the NOTICE file distributed with
  this work for additional information regarding copyright ownership.
  The ASF licenses this file to You under the Apache License, Version 2.0
  (the "License"); you may not use this file except in compliance with
  the License.  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
-->
# Apache Solr Operator — Threat Model (v0 DRAFT)

## §1 Header

- **Project:** Apache Solr Operator (`apache/solr-operator`) — the official
  Kubernetes operator for Apache Solr. A kubebuilder/controller-runtime Go
  controller that watches `solr.apache.org/v1beta1` custom resources
  (`SolrCloud`, `SolrBackup`, `SolrPrometheusExporter`) and reconciles the
  Kubernetes objects (StatefulSets, Services, ConfigMaps, Secrets, Ingresses,
  PodDisruptionBudgets, ZookeeperClusters, PVCs) that make up a running Solr
  deployment. It is **not** the Solr search server — the search server is
  modelled separately in `apache/solr`'s `THREAT_MODEL.md`, cross-referenced
  throughout.
- **Written against:** `main` @ HEAD (2026-07).
- **Author:** ASF Security team, via the threat-model-producer rubric
  (Scovetta rubric), as a starting point for the Solr PMC.
- **Status:** **v0 DRAFT** — produced by the ASF Security team *for the Solr
  PMC to review, correct, own, and ratify*. **Reviewed by Houston Putman
  (Solr PMC) on 2026-07-15; his corrections are folded in.** Not yet
  ratified/merged. Remaining unconfirmed claims below are *(inferred)* and
  routed to a §14 open question; do not treat those as a maintainer position
  until the PMC has confirmed them.
- **Version binding:** once ratified, versioned alongside the operator. A
  report against operator version *N* is triaged against the model as it stood
  at *N*, not at HEAD.
- **Canonical role (proposed):** intended to be the operator's
  **scanner-facing** security model — the authoritative basis for triaging an
  automated finding as in- or out-of-model. It is deliberately *not* an
  operator handbook; the operator's user guide (docs/) and Solr's own
  "Securing Solr" guidance remain the human source of truth for deployment.
  This role split is a §14 question for the PMC to confirm.
- **Reporting cross-reference:** §8-violating findings via the ASF security
  process (report to `security@solr.apache.org`, see
  <https://solr.apache.org/security.html>); §3/§9 findings closed citing this
  document. The Solr Operator has no separate disclosure channel from Solr —
  §14 confirms.
- **Provenance legend:** *(documented)* = stated in the repo's code, CRD field
  docs, or the operator user guide (cited); *(maintainer)* = confirmed by a
  Solr PMC member in response to this draft (**Houston Putman, 2026-07-15**);
  *(inferred)* =
  reasoned from code/CRD/RBAC structure, not yet confirmed — each has a
  matching §14 open question.
- **Draft confidence:** ~14 documented / 0 maintainer / 27 inferred. This is a
  v0: the *(documented)* claims are lifted from CRD field comments, the RBAC
  manifests, and the user guide; everything about *intent, contract, and
  scope* is still hypothesis awaiting the PMC.

**What it is.** The Solr Operator is a **cluster-scoped Kubernetes
controller**. An operator/admin installs it once (typically Helm), granting it
a `ClusterRole`; from then on any user who can create a `SolrCloud` (or
`SolrBackup` / `SolrPrometheusExporter`) custom resource in a watched
namespace causes the operator — acting with its own cluster-wide privileges —
to create and manage the backing Kubernetes objects and to make authenticated
HTTP calls into the Solr pods it manages. The defining security fact is that
the operator is a **privileged intermediary**: it turns a namespaced,
relatively low-privilege action (creating a CR) into high-privilege cluster
mutations (creating Secrets, exec-ing into pods, managing StatefulSets),
across the namespaces it watches (all namespaces by default, though it can
equally be installed namespace-scoped, watching only specific namespaces).

## §2 Scope and intended use

The Solr Operator is a **long-running in-cluster controller**, not a library
and not a user-facing service. Roles:

- **CRD author / namespace tenant** — a user (human or CI identity) with RBAC
  permission to create/update `SolrCloud`, `SolrBackup`,
  `SolrPrometheusExporter` objects in a watched namespace. **Semi-trusted at
  most**: they author the *desired state* but do not themselves hold the
  operator's cluster privileges. This is the primary adversary of interest
  (§7). *(inferred — Q-tenant.)*
- **Cluster operator / admin** — installs the operator, grants its RBAC
  (a cluster-wide `ClusterRole`, or namespace-scoped `Role`s when the operator
  watches specific namespaces), chooses watched namespaces, controls the Helm
  values. **Fully trusted** — owns the operator's blast radius.
  *(maintainer — ClusterRole/Role distinction confirmed by HoustonPutman
  2026-07-15.)*
- **The managed Solr (search server) and its clients** — out of scope here;
  modelled by `apache/solr`'s `THREAT_MODEL.md`. The operator is a *client* of
  Solr (it makes authenticated admin API calls), not the thing serving
  queries.

**Component families.**

| Family | Entry point | Privilege / exposure | In model? |
| --- | --- | --- | --- |
| **CRD field handling** (`api/v1beta1`) | `SolrCloud`/`SolrBackup`/`SolrPrometheusExporter` spec | tenant-authored desired state consumed by a privileged controller | **Yes (primary)** |
| **Reconciliation controllers** (`controllers/`) | watch → reconcile loop | acts with the operator ServiceAccount's cluster RBAC | **Yes (primary)** |
| **Operator RBAC** (`config/rbac/role.yaml`) | the `ClusterRole` (or namespace-scoped `Role`s) bound to the operator SA | secrets/pods-exec/statefulsets across watched namespaces | **Yes (blast radius)** |
| **Secret provisioning** (`controllers/util/solr_security_util.go`) | bootstrap basic-auth + `security.json` secrets | generates & stores Solr admin credentials | **Yes** |
| **TLS handling** (`solr_tls_util.go`, `main.go` client cert) | mounted/provisioned certs; operator→Solr mTLS client | trust of Solr server certs | **Yes** |
| **Backup subsystem** (`SolrBackup`, `solr_backup_repo_util.go`) | repo config + cloud credentials (S3/GCS/volume) | reads backup-repo credential secrets | **Yes** |
| **ZooKeeper coupling** | provisions `ZookeeperCluster` (pravega), ZK ACL secrets | cluster state/config store for SolrCloud | **Yes (trust ZK provider)** |
| Admission / validation webhooks | — | — | **No — none shipped** (§3) |
| **The managed Solr search server** | Solr HTTP API | query/index/admin | **No — `apache/solr` model** (§3) |
| Kubernetes control plane (API server, kubelet, etcd) | — | the platform | **No** (§3) |

## §3 Out of scope (explicit non-goals)

- **Apache Solr itself (the search server).** Everything about the Solr HTTP
  API — query/update/admin surface, SSRF via `shards`/streaming, Solr auth/
  authz plugin correctness, RCE via risky Solr features — is modelled by
  `apache/solr`'s `THREAT_MODEL.md`. This model covers only how the *operator*
  configures, credentials, and talks to Solr. *(documented — separate repo;
  Q-scope.)*
- **The Kubernetes control plane and platform.** The API server, admission
  control, RBAC enforcement engine, kubelet, etcd, the CNI, and the node OS
  are the platform's trust base. The operator assumes k8s enforces the RBAC it
  is granted. A finding whose precondition is "the k8s API server mis-enforces
  RBAC" or "etcd is readable" is out of model. *(inferred — Q-platform.)*
- **Cluster-admin as adversary.** Anyone who can edit the operator's
  `ClusterRole`, read arbitrary Secrets cluster-wide, or `exec` into the
  operator pod has already won; they are not a meaningful adversary to model
  at this layer. *(inferred — Q-admin.)*
- **The ZooKeeper implementation.** The operator provisions a
  `ZookeeperCluster` via the external pravega zookeeper-operator (a separate
  project) and trusts it. Securing/patching ZooKeeper and that operator is out
  of model. *(inferred — Q-zk.)*
- **Third-party dependency CVEs** (controller-runtime, client-go, the
  zookeeper-operator, container base images) — case-by-case: in model only
  when there is a reasonable operator-side mitigation; otherwise routed to the
  dependency's own project.
- **Code shipped for development/testing** — `tests/`, `hack/`, `example/`,
  `dev-docs/`, and `*_test.go`. Unsupported for production; threat-model
  separately if ever promoted. *(inferred — Q-scope.)*
- **Pass-through Kubernetes options on dependent resources.** Custom
  Kubernetes options a tenant sets that the operator copies verbatim onto the
  dependent objects it creates (StatefulSet, Service, Pod, etc.) are native
  Kubernetes fields passed straight through, so their behaviour and security
  semantics are Kubernetes's, not the operator's. *(maintainer — confirmed by
  HoustonPutman 2026-07-15.)*

## §4 Trust boundaries and data flow

There are **two** trust boundaries, and conflating them is the most common
triage error for an operator:

1. **Tenant → operator (the CR boundary).** A namespace tenant writes a
   `SolrCloud`/`SolrBackup`/`SolrPrometheusExporter` spec. The operator reads
   that spec and acts on it *with the operator ServiceAccount's cluster
   privileges*, not the tenant's. This is the boundary this model cares about
   most: fields in the CR are **attacker-controllable input to a privileged
   process** (§6).
2. **Operator → Solr (the HTTP boundary).** The operator is an authenticated
   *client* of the Solr pods it manages (basic-auth as `k8s-oper`, optionally
   over mTLS). Trust flows the other way here: the operator trusts the Solr
   server it dials (see the `tls-skip-verify-server` default, §5a).

```
tenant ─(create/patch CR)─► [k8s API + RBAC] ─► operator reconcile loop
                                                    │ acts as operator SA (ClusterRole)
                                                    ├─► create/patch Secrets, ConfigMaps, Services
                                                    ├─► create/patch StatefulSets, Deployments, PDBs, Ingress, PVCs
                                                    ├─► pods/exec (create) into managed pods
                                                    └─► provision ZookeeperCluster (pravega CRD)
operator ─(HTTPS admin API, basic-auth k8s-oper, optional mTLS)─► managed Solr pods
   (TLS server-cert verification OFF by default: tls-skip-verify-server=true)
```

**Reachability precondition (triager's test).** A finding is in-model only if
it is reachable by a **CRD author who is not a cluster-admin**, and the effect
crosses from the tenant's authority into the operator's cluster authority in a
way the tenant should not be able to reach — e.g. a CR field that makes the
operator read a Secret the tenant could not read directly, mount an arbitrary
volume, or exec somewhere it should not. A finding that merely requires
cluster-admin, or that lands in the Solr search server, is out of model
(§3). *(inferred — Q-tenant/Q-reach.)*

## §5 Assumptions about the environment

- **Kubernetes enforces RBAC and namespace isolation.** The operator assumes
  the API server correctly gates who may create CRs and correctly scopes the
  operator's own `ClusterRole`. *(inferred — Q-platform.)*
- **The operator runs with a cluster-scoped ServiceAccount** granting the
  `config/rbac/role.yaml` `ClusterRole`; by default it **watches all
  namespaces** (`watchNamespaces: ""` in Helm values → whole-cluster cache and
  reconcile). *(documented — `helm/solr-operator/values.yaml`, `main.go`
  `watch-namespaces` flag; Q-scope-default.)*
- **Only one active controller** via leader election
  (`leader-elect` defaults true). *(documented — `main.go`.)*
- **The pravega ZooKeeper operator** is present when SolrCloud provisions its
  own ZK. *(inferred — Q-zk.)*
- **Secrets live in the same namespace** as the SolrCloud/exporter for
  credentials the operator reads (e.g. ZK ACL secret refs are documented as
  same-namespace). *(documented — `common_types.go` ZookeeperACL comment.)*
- The operator opens outbound HTTPS to managed Solr pods and to the k8s API;
  it does not expose an inbound service of its own beyond metrics/health.
  *(inferred — Q-listeners.)*

## §5a Configuration variants — the knobs that move the model

1. **`watchNamespaces` (Helm `watchNamespaces`, `--watch-namespaces`)** —
   **default empty = watch and reconcile every namespace** with a single
   cluster-scoped `ClusterRole`. This maximises blast radius: a CR author in
   *any* namespace can drive the privileged operator. Restricting to an
   explicit namespace list (and narrowing the RBAC to `Role`/`RoleBinding`) is
   the hardened posture the code comments themselves recommend. *(documented —
   `main.go` comments explicitly suggest per-namespace operators + narrowed
   RBAC; Q-scope-default.)*
2. **`tls-skip-verify-server` (`main.go`, default `true`)** — the operator's
   HTTP client that calls Solr **does not verify the Solr server's
   certificate chain or hostname by default** ("insecure … accepts any
   certificate"). Operator→Solr traffic is therefore not authenticated
   server-side unless the admin sets this false and supplies a CA. *(documented
   — `main.go` flag help text; Q-tls-default.)*
3. **`SolrCloud.spec.solrSecurity` absent (default)** — if the CR author does
   not set `solrSecurity`, the operator provisions Solr **with no
   authentication** (Solr's own insecure-default posture). Auth is opt-in per
   SolrCloud. *(documented — `authentication-and-authorization.adoc`; the
   managed-Solr consequence is `apache/solr`'s model; Q-auth-default.)*
4. **`solrSecurity.probesRequireAuth` (default `false`)** — probe endpoints
   are reachable unauthenticated by default; the bootstrapped `security.json`
   deliberately allows unauthenticated access to probe paths. *(documented —
   CRD comment + auth doc; Q-probes.)*
5. **Bootstrap `security.json` vs user-provided** — by default the operator
   generates a `security.json` and random passwords for `admin`, `solr`,
   `k8s-oper`; alternatively the tenant supplies `basicAuthSecret` +
   `bootstrapSecurityJson`. The generated path uses non-cryptographic
   randomness (§8/§9). *(documented — `solr_security_util.go`; Q-rng.)*
6. **RBAC creation (`rbac.create`, `serviceAccount.create` Helm, default
   true)** — the chart creates the cluster `ClusterRole`/binding by default.
   *(documented — `values.yaml`.)*

**Wave-1 ruling needed (Q-scope-default / Q-tls-default / Q-auth-default):**
for each insecure default above, confirm whether it is the *supported
production posture* (so a report against it is `VALID`) or a
dev-convenience the operator is documented to change (so it is
`OUT-OF-MODEL: non-default-config`).

## §6 Assumptions about inputs

The operator's primary untrusted input is **the CR spec**, authored by a
namespace tenant and consumed by a privileged controller. There are **no
admission/validation webhooks** (§3), so field validation is limited to
kubebuilder OpenAPI schema constraints (enums, minimums) — anything not
schema-constrained is accepted as authored. *(inferred — Q-validation.)*

| Boundary / source | Input | Attacker (tenant) controllable? | Enforced by / who owns |
| --- | --- | --- | --- |
| `SolrCloud` spec | pod/container overrides, `customSolrKubeOptions`, volumes, env, image | **yes** | only OpenAPI schema; operator applies as-is to a privileged StatefulSet |
| `SolrCloud.spec.solrSecurity.basicAuthSecret` / `bootstrapSecurityJson` | secret name refs | **yes** | operator reads named Secret in the SolrCloud namespace |
| `SolrCloud.spec.zookeeperRef...acl.secret` | ZK ACL secret ref | **yes** | must be same-namespace (documented) |
| `SolrCloud` TLS options | keystore/truststore secret refs, mounted cert paths | **yes** | operator mounts/reads referenced secrets |
| `SolrBackup` spec | `repositoryName`, `location`, repo credentials | **yes** | operator reads backup-repo credential secrets (S3/GCS/volume) |
| `SolrPrometheusExporter` spec | exporter config, image, scrape target | **yes** | applied to a Deployment |
| operator → Solr | Solr server TLS certificate | Solr side | **not verified by default** (`tls-skip-verify-server`) |
| operator → k8s API | watch/list responses (CRs, Secrets) | k8s-mediated | trusted (RBAC-scoped) |

The central §14 question is which of these tenant-controllable fields can be
used to make the operator's cluster privileges do something the tenant could
not do directly — e.g. does the operator constrain which Secrets / volumes /
hostPaths a CR may reference to the CR's own namespace, or can a CR reference
resources elsewhere? *(inferred — Q-crossns / Q-podspec.)*

## §7 Adversary model

- **Namespace tenant / CRD author (primary).** Holds RBAC to create/patch
  `SolrCloud`/`SolrBackup`/`SolrPrometheusExporter` in a watched namespace,
  but does **not** hold the operator's cluster privileges. In scope: they try
  to craft a CR that makes the privileged operator read a Secret they could
  not read, mount a volume/hostPath they should not, exec into or co-opt a pod
  they do not own, or otherwise escalate out of their namespace. *(inferred —
  Q-tenant.)*
- **Compromised managed Solr pod (secondary).** A Solr pod the operator dials.
  In scope to the extent that, because `tls-skip-verify-server` defaults on,
  an on-path actor or a rogue endpoint could impersonate Solr to the operator
  and harvest the `k8s-oper` credentials the operator presents. *(inferred —
  Q-tls-default.)*
- **Out of scope:** cluster-admin (can edit the operator's RBAC — already
  won); the k8s control plane; the pravega ZK operator; the Solr search
  server's own client-facing adversaries (that is `apache/solr`'s model); an
  attacker who already holds the operator ServiceAccount token.

## §8 Security properties the project provides

Each property: statement · violation symptom · severity · provenance.

1. **Privilege intermediation is mediated only by k8s RBAC.** The operator
   acts on a CR strictly within the resource kinds/verbs its `ClusterRole`
   grants; it does not escalate beyond them. *Violation:* the operator
   performs a cluster action outside its granted RBAC, or a CR field causes an
   action the operator's own model did not intend (a *confused-deputy* across
   namespaces). *Severity:* critical. *(inferred — Q-tenant/Q-crossns.)*
2. **Per-SolrCloud credential isolation.** Bootstrap basic-auth /
   `security.json` secrets are generated per SolrCloud, owned via
   `OwnerReference`, and stored in the SolrCloud's namespace; passwords are
   randomized per SolrCloud. *Violation:* one tenant's SolrCloud credentials
   are readable by, or reused across, another tenant. *Severity:* high.
   *(documented — `solr_security_util.go`, auth doc; Q-secret-scope.)*
3. **The operator does not persist Solr admin credentials beyond the
   bootstrap secret.** Once `security.json` is bootstrapped the operator does
   not update it and only holds the `k8s-oper` read-only credential for its own
   calls. *Violation:* operator logs or status leak admin/solr passwords.
   *Severity:* high. *(documented — auth doc "operator will not update it",
   `k8s-oper` read-only; Q-credleak.)*
4. **TLS to Solr is supported for client and inter-node traffic.** The
   operator can provision/consume keystores and present a client cert (mTLS)
   to Solr. *Violation:* the operator cannot be configured to use TLS where
   the CR requests it, or downgrades silently. *Severity:* medium (the
   *default* is the weaker posture — see §9). *(documented — `solr_tls_util.go`,
   `tls.adoc`; Q-tls-default.)*
5. **Leader election ensures a single active reconciler.** *Violation:* two
   controllers fight over the same resources, producing divergent state.
   *Severity:* correctness (availability). *(documented — `main.go`.)*
6. **Reconcile convergence / desired-state correctness.** The managed objects
   converge to the CR spec. *Violation:* drift or wrong object generated.
   *Severity:* correctness, unless it crosses a security boundary (then see
   property 1). *(inferred — Q-correctness.)*

## §9 Security properties the project does *not* provide

- **No admission/validation webhook.** The operator ships **no** validating or
  mutating admission webhook (no `config/webhook`, no webhook entries in
  `PROJECT`). Cross-field or policy validation of a CR that OpenAPI schema
  cannot express is **not** enforced; malformed-but-schema-valid specs are
  accepted. If you need to constrain what tenants may put in a `SolrCloud`
  (images, volumes, hostPaths, security context), that is the cluster's
  admission-policy responsibility (Gatekeeper/Kyverno/PSA), not the operator's.
  *(documented — absence in repo; Q-validation.)*
  - *False friend:* the CRD's OpenAPI schema (enums like
    `authenticationType: Basic`, `+kubebuilder:validation:Minimum`) looks like
    input validation but only enforces shape, not policy or trust.
- **The operator does not verify Solr's TLS server certificate by default.**
  `tls-skip-verify-server` defaults to `true` — operator→Solr HTTPS accepts
  any certificate. This is a confidentiality/authentication gap for the
  operator's own admin-API calls (it presents `k8s-oper` credentials to
  whatever answers). *(documented — `main.go`; Q-tls-default.)*
- **Bootstrap passwords are not generated with a cryptographic RNG.** The
  bootstrap `admin`/`solr`/`k8s-oper` passwords are produced with `math/rand`
  seeded from `time.Now().UnixNano()` (and the salt via `math/rand`'s
  `rand.Read`), not `crypto/rand`. Treat the generated passwords as
  convenience bootstrap credentials, not high-entropy secrets; rotate via the
  Solr Security API. *(inferred — `solr_security_util.go`; Q-rng.)*
  - *False friend:* "the operator generates a random password" reads as
    cryptographically strong; it is not, by construction.
- **The operator does not author your Kubernetes authorization.** Who may
  create a `SolrCloud`, and how broad the operator's `ClusterRole` is, are the
  cluster-admin's decisions. An over-broad grant (operator watching all
  namespaces with cluster secrets access) is an install-time choice (§5a/§10).
- **No tenant isolation beyond what k8s gives you.** The operator does not
  add a second authorization layer over the CR API; if two teams can create
  CRs in the same watched scope, the operator does not partition them.
  *(inferred — Q-tenant.)*
- **No protection of ZooKeeper, the container images, or the node.** (§3.)
- **Well-known attack classes the operator itself cannot fully close and
  leaves to the cluster:** confused-deputy / cross-namespace secret or volume
  reference via CR fields; privileged-pod or hostPath escape via unconstrained
  pod-spec overrides; credential harvest via unverified TLS to a spoofed Solr;
  supply-chain of the managed container images. One sentence each — the point
  is to put the integrator on notice.

## §10 Downstream responsibilities (cluster operator / admin)

- **Scope the operator down.** Prefer per-namespace operators with a narrowed
  `Role`/`RoleBinding` over the default all-namespaces `ClusterRole`; the
  code comments themselves recommend this. Restrict who may create Solr CRs
  via RBAC. *(documented — `main.go`; Q-scope-default.)*
- **Constrain CR content with cluster admission policy.** Because there is no
  operator webhook (§9), use Pod Security Admission / Gatekeeper / Kyverno to
  bound images, volumes, hostPaths, and security contexts that tenants can
  request through a `SolrCloud`.
- **Set `tls-skip-verify-server=false` and supply a CA** if operator→Solr
  traffic must be authenticated/confidential; enable TLS for client and
  inter-node Solr traffic. *(documented — `main.go`, `tls.adoc`.)*
- **Enable Solr auth per SolrCloud** (`solrSecurity`) rather than relying on
  the unauthenticated default; rotate the bootstrap `admin`/`solr` passwords
  via the Solr Security API and treat the generated ones as low-entropy
  bootstrap values. Delete the bootstrap secret after capturing the admin
  password. *(documented — auth doc.)*
- **Secure ZooKeeper and its ACLs** for SolrCloud; provide ZK ACL secrets in
  the correct namespace.
- **Protect backup-repository credentials** (S3/GCS/volume secrets) and the
  buckets/paths they grant.
- Keep the operator, controller-runtime, and the managed container images
  patched.

## §11 Known misuse patterns

- **Running the operator cluster-wide with broad RBAC in a multi-tenant
  cluster** and letting many teams create Solr CRs, so the operator becomes a
  shared confused-deputy across namespace boundaries.
- **Leaving `tls-skip-verify-server` at its insecure default** on a network
  where the operator→Solr path is not otherwise protected, exposing the
  `k8s-oper` credentials to a spoofed endpoint.
- **Relying on the bootstrap-generated passwords long-term** instead of
  rotating them, given their non-cryptographic origin.
- **Deploying SolrCloud with no `solrSecurity`** and exposing the resulting
  unauthenticated Solr (this then becomes the `apache/solr` model's #1 misuse).
- **Expecting the operator to validate/PSP-gate tenant pod specs** — it does
  not (no webhook); unconstrained overrides can request privileged/hostPath
  pods.

## §11a Known non-findings (recurring false positives)

- **"The operator has broad cluster RBAC (secrets, pods/exec, statefulsets
  cluster-wide)"** — that is the operator's designed authority, granted at
  install by a cluster-admin. `BY-DESIGN` unless a *tenant* (non-admin) can
  make the operator use it outside the tenant's own namespace/authority
  (then §8 property 1, possibly `VALID`). (§8/§9.)
- **"`pods/exec` create permission"** — used for legitimate cluster-operation
  commands against managed Solr pods; a designed capability, not an escalation
  on its own. `BY-DESIGN`. (§8.)
- **"No admission webhook / weak CR validation"** — accurate but by design;
  policy validation is the cluster's job (§9/§10). `BY-DESIGN:
  property-disclaimed`, or `VALID-HARDENING` if a specific missing check makes
  a §11 misuse trivially easy.
- **"Solr is unauthenticated by default" / "SSRF/RCE in Solr"** — that is the
  managed search server, `OUT-OF-MODEL: separate-component`; route to
  `apache/solr`'s model. (§3.)
- **"Uses `math/rand` for passwords"** — a real hardening item (tracked in §9);
  triage as `VALID-HARDENING` unless the PMC rules the bootstrap credentials
  are not intended to be cryptographic secrets, in which case
  `BY-DESIGN`. This is a §14 question, not a settled non-finding.
- **Findings in `tests/`, `hack/`, `example/`** — `OUT-OF-MODEL:
  unsupported-component` (§3).
- **Third-party operator/dependency issues (pravega ZK operator,
  controller-runtime)** — route to the dependency's project unless there is an
  operator-side mitigation.

## §12 Conditions that would change this model

- Adding an admission/validation webhook (changes §6/§9 materially).
- Changing an insecure default: `watchNamespaces`, `tls-skip-verify-server`,
  the auth/probe defaults, or the bootstrap RNG.
- A new CRD or a new tenant-controllable field that reaches a privileged
  action (new confused-deputy surface).
- Supporting new backup repositories or credential sources.
- The operator taking on any inbound request surface of its own beyond
  metrics/health.
- A report that cannot be routed to a §13 disposition → revise §8/§9.

## §13 Triage dispositions

| Disposition | Meaning | Licensed by |
| --- | --- | --- |
| `VALID` | A §8 property breaks for an in-scope actor — chiefly a non-admin tenant escalating out of their namespace via a CR field. | §8, §6, §7 |
| `VALID-HARDENING` | No §8 break, but a §11 misuse (e.g. missing CR-field constraint, weak-RNG passwords) is easy enough to warrant hardening. | §11 |
| `OUT-OF-MODEL: trusted-input` | Requires control of install-time config the model marks trusted (Helm values, the `ClusterRole`). | §6/§10 |
| `OUT-OF-MODEL: adversary-not-in-scope` | Requires cluster-admin, control-plane, or ZK-operator compromise. | §7 |
| `OUT-OF-MODEL: non-default-config` | Only manifests under a discouraged/opted-in setting, once the PMC confirms which defaults are supported vs dev-only. | §5a |
| `OUT-OF-MODEL: separate-component` | Lands in the Solr search server, the pravega ZK operator, or another repo's model. | §3 |
| `OUT-OF-MODEL: unsupported-component` | `tests/`, `hack/`, `example/`, `*_test.go`. | §3 |
| `BY-DESIGN: property-disclaimed` | Concerns a property §9 disclaims (broad RBAC by design, no webhook, unverified TLS default before ruling). | §9 |
| `KNOWN-NON-FINDING` | Matches §11a. | §11a |
| `MODEL-GAP` | Cannot be routed → revise the model. | triggers §12 |

## §14 Open questions for the maintainers

Every *(inferred)* tag above routes to one of these. Proposed answers are
stated for the PMC to confirm, correct, or strike. This is a v0, so the list
is long by design.

**Wave 1 — scope, roles, and the load-bearing defaults.**

- **Q-scope.** Confirm the scope split: this model covers only the operator;
  the Solr search server, the pravega ZK operator, and `tests/`/`hack/`/
  `example/` are out (routed to their own models / unsupported). (§2/§3.)
- **Q-tenant / Q-admin.** Confirm the two-actor model: a **namespace tenant /
  CR author** is the primary (semi-trusted) adversary and a **cluster-admin**
  is fully trusted and out of scope. Is a CR author expected to be trusted or
  untrusted in the deployments you design for? (§2/§7.)
- **Q-scope-default.** Is the default **all-namespaces** watch with a
  cluster-wide `ClusterRole` the *supported production posture* (so
  cross-namespace confused-deputy findings are `VALID`), or is per-namespace
  scoping the expected production install (making all-namespaces
  `OUT-OF-MODEL: non-default-config`)? (§5a/§8.)
- **Q-tls-default.** Is `tls-skip-verify-server=true` (no Solr server-cert
  verification) the supported default, or a dev-convenience operators must
  flip? A report of operator-credential exposure to a spoofed Solr — `VALID`
  or `OUT-OF-MODEL: non-default-config`? (§5a/§8/§9.)
- **Q-auth-default.** Is provisioning Solr with **no** `solrSecurity`
  (unauthenticated) a supported operator posture, or is it expected that
  production SolrClouds always set `solrSecurity`? (§5a.)

**Wave 2 — the confused-deputy surface (the crux).**

- **Q-crossns / Q-podspec.** For each tenant-controllable CR field that names
  a Kubernetes object (secret refs, volume sources, ZK ACL secret, TLS
  keystore secrets, backup-repo credential secrets) or overrides the pod spec
  (`customSolrKubeOptions`, volumes, security context, image): does the
  operator constrain these to the CR's own namespace and to non-privileged
  shapes, or can a CR author make the operator read/mount something they could
  not access directly? This determines most `VALID` vs `BY-DESIGN` calls.
  (§6/§8.)
- **Q-validation.** Confirm there is no admission/validation webhook and that
  CR validation is intentionally left to OpenAPI schema + cluster admission
  policy (PSA/Gatekeeper/Kyverno). (§6/§9.)
- **Q-secret-scope / Q-credleak.** Confirm per-SolrCloud credential isolation
  and that admin/solr passwords are never written to logs/status. (§8.)

**Wave 3 — credentials, TLS, and platform.**

- **Q-rng.** The bootstrap `admin`/`solr`/`k8s-oper` passwords and salt use
  `math/rand` seeded by time, not `crypto/rand`. Is this a hardening item to
  fix (`VALID-HARDENING`), or are these deemed low-value bootstrap credentials
  expected to be rotated, making it `BY-DESIGN`? (§8/§9/§11a.)
- **Q-zk.** Confirm ZooKeeper (and the pravega operator) is a trusted,
  out-of-model dependency, and that ZK ACL security is the operator's
  responsibility. (§3/§5.)
- **Q-platform / Q-listeners / Q-correctness.** Confirm the k8s control plane
  is trusted; confirm the operator exposes no inbound surface beyond
  metrics/health; confirm reconcile correctness is a correctness (not
  security) property unless it crosses a boundary. (§5/§8.)

**Wave 4 — meta / ownership.**

- **Q-doc.** Confirm the disclosure channel (`security@solr.apache.org`,
  shared with Solr) and the document's role: scanner-facing canonical model,
  complementary to the operator user guide. Should this `THREAT_MODEL.md` live
  in `apache/solr-operator` and be linked from an `AGENTS.md`/`SECURITY.md`, as
  the Solr search-server model is? (§1/§15.)

## §15 Appendix — document roles and source map

**Document roles (proposed; awaiting PMC).** This `THREAT_MODEL.md` is intended
as the operator's **scanner-facing** security model — the authoritative basis
for triaging automated findings against the operator (in/out-of-scope, the
§8/§9 property lists, §11a non-findings, §13 dispositions). It is deliberately
*not* an operator handbook. The human sources of truth remain:

- **Solr Operator user guide** (authentication/authorization, TLS,
  ZooKeeper, backup) — the `docs/` Antora site, e.g.
  `docs/modules/solr-cloud/pages/authentication-and-authorization.adoc` and
  `.../tls.adoc`.
- **Apache Solr search-server threat model** — `apache/solr`'s
  `THREAT_MODEL.md` (the managed-Solr surface: query/update/admin APIs, SSRF,
  Solr auth/authz, risky features). This operator model cross-references it for
  everything about the search server itself.
- **Securing Solr** (Solr Reference Guide) —
  <https://solr.apache.org/guide/solr/latest/deployment-guide/securing-solr.html>
- **Security advisories + reporting** — <https://solr.apache.org/security.html>.

**Source map (what grounded this v0).** The *(documented)* claims are lifted
from: `config/rbac/role.yaml` (the operator `ClusterRole`); `main.go`
(`watch-namespaces`, `leader-elect`, `tls-skip-verify-server` defaults, the
mTLS client cert plumbing); `helm/solr-operator/values.yaml` (`watchNamespaces`,
`rbac.create`, `serviceAccount.create`); `api/v1beta1/solrcloud_types.go`
(`SolrSecurityOptions`: `authenticationType`, `basicAuthSecret`,
`probesRequireAuth`, `bootstrapSecurityJson`); `api/v1beta1/common_types.go`
(`ZookeeperACL` same-namespace secret refs); `controllers/util/solr_security_util.go`
(bootstrap secret generation, `math/rand` passwords/salt, per-SolrCloud
randomization, "operator will not update security.json"); and
`docs/modules/solr-cloud/pages/authentication-and-authorization.adoc`
(k8s-oper read-only, random admin/solr passwords, probe-auth default). The
absence of `config/webhook` and of webhook entries in `PROJECT` grounds the
"no admission webhook" claim. Everything about *intent and contract* is
*(inferred)* and routed to §14 for the PMC.
