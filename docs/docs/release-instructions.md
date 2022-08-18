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