# The Helm Repository

The Solr Operator Helm charts are distributed as OCI artifacts on the GitHub Container Registry:

- `oci://ghcr.io/apache/solr-operator/helm/solr-operator`
- `oci://ghcr.io/apache/solr-operator/helm/solr`

The classic Helm repository, `helm repo add apache-solr https://solr.apache.org/charts`, is kept
as well. It is an `index.yaml` checked into the [solr-site](https://github.com/apache/solr-site)
repository, at `content/charts/index.yaml`. Its entries point at the OCI copies of the charts
rather than at tarballs, so no charts are hosted on the website:

```yaml
entries:
  solr-operator:
    - version: 0.10.0
      urls:
        - oci://ghcr.io/apache/solr-operator/helm/solr-operator:0.10.0
      digest: <sha256 of the released chart tarball>
      ...
```

The signed chart tarballs on dist.apache.org / archive.apache.org remain the official release
artifacts. The OCI copies are byte-identical to them, which is why the `digest` in the index
matches both, and why provenance (`--verify`) works through the index.

## Helm version support

An index with `oci://` chart urls needs Helm 3.8 or later.
[`test_oci_index.sh`](../hack/release/smoke_test/test_oci_index.sh) checks every way of consuming
the index (repo add/update, search, show, pull with and without `--verify`, install-style chart
resolution, and `helm dependency update` with a repository url or an `@alias`). It runs against
Helm 3.8.2, 3.19.0 and 4.3.1, in the `Helm OCI Index` GitHub workflow, and locally with:

```bash
make yq
./hack/release/smoke_test/test_oci_index.sh            # or -H "<helm versions>" to test others
```

It needs Docker, since the registry, the index server and each Helm version run in containers.

## Updating the index

[`update_helm_index.sh`](../hack/release/upload/update_helm_index.sh) adds a release to the index.
It downloads the released charts, verifies their checksums and GPG signatures, writes entries
pointing at the OCI registry, and signs the index. It never commits or uploads anything.

A release does this in the release wizard's `website_update_helm_index` step, after the charts are
published to the OCI registry (`publish_helm_charts_oci`), and the website change is published with
the rest of the release's website updates.

If a version is already in the index, its entry is replaced in place, keeping its original
`created` timestamp. The script refuses to do this if the chart's digest has changed, since the
OCI copy must be byte-identical to the released chart.

## Backfilling older releases

Releases from before the switch to OCI are listed in the index with relative tarball urls, which
the website redirects to nightlies.apache.org. To move one of them to OCI:

1. Publish it to the OCI registry, by running the
   [Publish Helm Charts to OCI Registry](https://github.com/apache/solr-operator/actions/workflows/publish-helm-oci.yaml)
   workflow with its archive.apache.org chart urls, e.g.
   `https://archive.apache.org/dist/solr/solr-operator/v0.9.1/helm-charts/solr-operator-0.9.1.tgz`.
   Use a dry run first.
2. Rewrite its index entries, in a solr-site checkout:
   ```bash
   ./hack/release/upload/update_helm_index.sh \
       -r https://archive.apache.org/dist/solr/solr-operator/v0.9.1 \
       -i ../solr-site/content/charts/index.yaml
   ```
3. Commit the updated `index.yaml`, `index.yaml.asc` and `index.yaml.sha512` in solr-site.

Versions from before the donation to Apache (`v0.2.x`) have no archive.apache.org copy, so they
are left as they are.

Once every entry points at OCI, the `/charts/*.tgz` redirect to nightlies in solr-site's
`themes/solr/templates/htaccess.template` can be removed.
