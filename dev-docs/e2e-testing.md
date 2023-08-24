# E2E/Integration Testing

The Solr Operator "unit" tests cannot fully test the Solr Operator, they can only test interactions with the Kubernetes API Server.
Because of this, many features are untestable because they require communicating with a running SolrCloud cluster.

The E2E (End-to-end), or integration, test suite enables the project to fully test the operator on a real "KiND" Kubernetes cluster.
Thus every feature can be fully tested.

## How to run the tests

There is an easy `make` target to run the e2e tests with default parameters.

```bash
$ make e2e-tests
```

This will create a [KinD Cluster](https://kind.sigs.k8s.io/) to act as the Kubernetes cluster,
and install the Solr Operator via its [Helm Chart](../helm/solr-operator).

It will run the e2e-tests using parallel test-runners, and each test runner will use its own namespace.
The Solr Operator will be deployed to listen on all namespaces.

If all tests succeed, then the KinD Cluster will be deleted.
Please [see below](#test-failures) for information on test failures.

The following sections describe all the ways for you to customize the e2e tests.

### Customizing the test parameters

The Solr Operator integration tests are meant to be customizable to test the operator in a range of use cases.
Currently you have to use a KinD cluster, but you can still change the Kubernetes Version and Solr Image to test with.
Future iterations will hopefully enable testing with existing clusters.

Beyond changing the Solr Image and Kubernetes environment, the test suite itself can be customized
for parallelization and randomization.

Example:
```bash
$ make e2e-tests TEST_SEED=89724023 SOLR_IMAGE=apache/solr-nightly:10.0.0-SNAPSHOT KUBERNETES_VERSION=v1.26.4
```

**Options**
- **TEST_SEED** - Equivalent to Ginkgo's `--seed`.
  If set, randomization in the test framework will be seeded with this number.
- **TEST_PARALLELISM** - Equivalent to Ginkgo's `--procs`.
  Ginkgo will use this many parallel test runners.
  The default parallelism is `3`.
- **SOLR_IMAGE** - The solr docker image label to use in the integration tests.
  It is recommended to use only supported versions for the Solr Operator version being tested.
  Default is `solr:8.11`.
- **KUBERETES_VERSION** - A full Kubernetes version, starting with `v`, to use when creating the KinD Cluster.
  To find a list of all possible versions, check the [KinD Node Docker tags](https://hub.docker.com/r/kindest/node/tags).
  Default is `v1.24.16`.

### Filtering tests

The full test suite might take too long if you just want to test a specific feature.
There are a number of ways to filter the tests that are run, and each can be specified via an environment variable.

**Options**
- **TEST_FILES** - Equivalent to Ginkgo's `--focus-file`.
  If set, tests will only run specs in matching files.
  Accepts: `[file (regexp) | file:line | file:lineA-lineB | file:line,line,line]`
- **TEST_FILTER** - Equivalent to Ginkgo's `--focus`.
  If set, only specs that match this regular expression will be run.  
  NOTE: The spec is the concatenation of all levels of hierarchy that an `It()` test belongs in.
  See below for more information.
- **TEST_LABELS** - Equivalent to Ginkgo's `--label-filter`.
  If set, only specs with labels that match the label-filter will be run.
  Note: A test has to have labels for this to be effective.  
  The passed-in expression can include boolean operations (`!`, `&&`, `||`, `,`),
  groupings via `()`, and regular expressions `/regexp/`.  e.g. `(cat || dog) && !fruit`
- **TEST_SKIP** - Equivalent to Ginkgo's `--skip`.
  Do not run tests that match this regular expression string.
  NOTE: The same rules on spec names from `TEST_FILTER` apply here.

#### Test/Spec name matching

```bash
$ make e2e-tests TEST_FILTER="E2E - Backups Local Directory - Recurring Takes a backup correctly"
```

The above test's hierarchy is:
- `Describe("E2E - Backups"`
- `Context("Local Directory - Recurring"`
- `It("Takes a backup correctly"`

Thus we concatenate all 3 together, with each name separated by a space, to get the unique test filter.

If `TEST_FILTER="E2E - Backups Local Directory - Recurring"` is used, then
all tests under the first two test `Describe` and `Context` will be run.

Since this is a regex string, using just `TEST_FILTER="Backups"` will match any `Describe`, `Context`, or `It`
that contains the work "Backups".

### Customizing the test environment

It is important to have control over the KinD cluster that is created/used when running the e2e tests.
The following options are aimed at opening this up.

**Options**
- **REUSE_KIND_CLUSTER_IF_EXISTS** - Defaults to `true`.
  If a kind cluster for the exact same setup (solr image, kube version, operator version),
  already exists, this option determines whether to use that cluster for the tests or to delete and recreate
  the cluster before tests are run.
- **LEAVE_KIND_CLUSTER_ON_SUCCESS** - Defaults to `false`.
  On test failures, the KinD cluster is not deleted afterwards, so that tests can be quickly rerun with the same environment.
  If this option is set to `true`, then the KinD cluster will not be deleted afterwards, even if all tests succeed.
  This might be useful when quickly iterating on tests that succeed, to reduce the time to create new clusters.

## Test Failures

If a test fails, Ginkgo will print out a command that will retest the individual test that failed.
If multiple tests in a run fail, then Ginkgo will print out a command for each failed test.
This command will also rerun the test with the same randomization seed that the test failed with.

If a test fails, the KinD Kubernetes Cluster will not be deleted.
This way the tests can be re-run using the same cluster that they failed on previously.
Use the [environment variable `REUSE_KIND_CLUSTER_IF_EXISTS=false`](#customizing-the-test-environment) to re-create the KinD cluster,
instead of reusing the existing one.

# IntelliJ & GoLand Testing

The e2e-tests can be tested via IDEA, just as the unit tests can,
though creating the cluster will make the tests significantly slower.
Please refer to the [IDEA Testing Docs](idea-tests.md) for more information.
