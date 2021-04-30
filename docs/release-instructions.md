# Releasing a New Verson of the Solr Operator

This page details the steps for releasing new versions of the Solr Operator.

- [Versioning](#versioning)
- [Use the Release Wizard](#release-wizard)
 
### Versioning

The Solr Operator follows kubernetes conventions with versioning with is:

`v<Major>.<Minor>.<Patch>`

For example `v0.2.5` or `v1.3.4`.
Certain systems except versions that do not start wth `v`, such as Helm.
However the tooling has been created to automatically make these changes when necessary, so always include the prefixed `v` when following these instructions.

### Release Wizard

Run the release wizard from the root of the repo on the branch that you want to make the release from.

```bash
./hack/release/wizard/releaseWizard.py
```

Make sure to install all necessary programs and follow all steps.
If there is any confusion, it is best to reach out on slack or the mailing lists before continuing.