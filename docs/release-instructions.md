# Releasing a New Verson of the Solr Operator

This page details the steps for releasing new versions of the Solr Operator.

- [Versioning](#versioning)
  - [Backwards Compatibility](#backwards-compatibility)
- [Create the Upgrade Commit](#create-the-upgrade-commit)
- [Create a release PR and merge into `master`](#create-a-release-pr-and-merge-into-master)
- [Tag and publish the release](#tag-and-publish-the-release)
 
### Versioning

The Solr Operator follows kubernetes conventions with versioning with is:

`v<Major>.<Minor>.<Patch>`

For example `v0.2.5` or `v1.3.4`.
Certain systems except versions that do not start wth `v`, such as Helm.
However the tooling has been created to automatically make these changes when necessary, so always include the prefixed `v` when following these instructions.

#### Backwards Compatibility

All patch versions of the same major & minor version should be backwards compatabile.
Non-backwards compatible changes will be allowed while the Solr Operator is still in a beta state.
 
### Create the upgrade commit

The last commit of a release version of the Solr Operator should be made via the following command.

```bash
$ VERSION=<version> make release
```

This will do the following steps:

1. Set the variables of the Helm chart to be the new version.
1. Build the CRDs and copy them into the Helm chart.
1. Package up the helm charts and index them in `docs/charts/index.yaml`.
1. Create all artifacts that should be included in the Github Release, and place them in the `/release-artifacts` directory.
1. Commits all necessary changes for the release.

### Create a release PR and merge into `master`

Now you need to merge the release commit into master.
You can push it to your fork and create a PR against the `master` branch.
If the Travis tests pass, "Squash and Merge" it into master.

### Tag and publish the release

In order to create a release, you can do it entirely through the Github UI.
Go to the releases tab, and click "Draft a new Release".

Follow the formatting of previous releases, showing the highlights of changes in that version nicluding links to relevant PRs.

Before publishing, make sure to attach all of the artifacts from the `release-artifacts` directory that were made when running the `make release` command earlier in the guide.

Once you publish the release, Travis should re-run and deploy the docker containers to docker hub.