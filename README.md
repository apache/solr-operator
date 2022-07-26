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

# Solr Operator
[![Latest Version](https://img.shields.io/github/tag/apache/solr-operator)](https://github.com/apache/solr-operator/releases)
[![License](https://img.shields.io/badge/LICENSE-Apache2.0-ff69b4.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/apache-solr)](https://artifacthub.io/packages/search?repo=apache-solr)
[![Commit since last release](https://img.shields.io/github/commits-since/apache/solr-operator/latest.svg)](https://github.com/apache/solr-operator/commits/main)
[![Docker Pulls](https://img.shields.io/docker/pulls/apache/solr-operator)](https://hub.docker.com/r/apache/solr-operator/)
[![Slack](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://kubernetes.slack.com/messages/solr-operator)

The __[Solr Operator](https://solr.apache.org/operator/)__ manages Apache Solr Clouds within Kubernetes.
It is built on top of the [Kube Builder](https://github.com/kubernetes-sigs/kubebuilder) framework.
Please visit the [official site](https://solr.apache.org/operator/) for more information.

The project is currently in beta (`v1beta1`), and while we do not anticipate changing the API in backwards-incompatible ways there is no such guarantee yet.

If you run into issues using the Solr Operator, please:
- Reference the [version compatibility and upgrade/deprecation notes](#version-compatibility--upgrade-notes) provided below
- Create a GitHub Issue in this repo, describing your problem with as much detail as possible
- Reach out on our Slack channel!

Join us on the [#solr-operator](https://kubernetes.slack.com/messages/solr-operator) channel in the official Kubernetes slack workspace.

## Menu

- [Documentation](#documentation)
- [Version Compatibility and Upgrade Notes](#version-compatibility--upgrade-notes)
- [Contributions](#contributions)
- [License](#license)
- [Code of Conduct](#code-of-conduct)
- [Security Vulnerability Reporting](#security-vulnerability-reporting)

## Documentation

Please visit the following pages for documentation on using and developing the Solr Operator:

- [Local Tutorial](https://apache.github.io/solr-operator/docs/local_tutorial)
- [Helm Instructions via Artifact Hub](https://artifacthub.io/packages/helm/apache-solr/solr-operator)
  - The released helm charts and their instructions should be used for all safe and stable deployments.
    The charts found in `helm/` are not guaranteed to be compatible with the last stable release, and should only be used for development purposes.
- [Running the Solr Operator](https://apache.github.io/solr-operator/docs/running-the-operator)
- Available Solr Resources
    - [Solr Clouds](https://apache.github.io/solr-operator/docs/solr-cloud)
    - [Solr Backups](https://apache.github.io/solr-operator/docs/solr-backup)
    - [Solr Metrics](https://apache.github.io/solr-operator/docs/solr-prometheus-exporter)
- [Development](https://apache.github.io/solr-operator/docs/development)

### Examples

Example uses of each CRD have been [provided](https://apache.github.io/solr-operator/example).

## Version Compatibility & Upgrade Notes

Make sure to check the [Solr Operator Upgrade notes](docs/upgrade-notes.md), before upgrading the Solr Operator or CRDs in your Kubernetes cluster.

This page also contains [Version Compatibility Matrixes](docs/upgrade-notes.md#version-compatibility-matrixes), which detail the compatible Solr versions and Kubernetes versions for each release of the Solr Operator.

## Contributions

We :heart: contributions.

Have you had a good experience with the **Solr Operator**? Why not share some love and contribute code, or just let us know about any issues you had with it?

We welcome issue reports [here](../../issues); be sure to choose the proper issue template for your issue, so that we can be sure you're providing the necessary information.

Before submitting a PR, please be sure to run `make prepare` before committing.
Otherwise the GitHub checks are likely to fail.

If you are trying to run tests locally in IntelliJ/GoLand, refer to [the IDEA tests docs](dev-docs/idea-tests.md).

## License

Please read the [LICENSE](LICENSE) file here.

### Docker Image Licenses

The Solr Operator docker image contains NOTICE and LICENSE information in the `/etc/licenses` directory.
This is different from the source release LICENSE and NOTICE files, so make sure to familiarize yourself when using the image.

## Code of Conduct

This space applies the ASF [Code of Conduct](https://www.apache.org/foundation/policies/conduct)
If you have any concerns about the Code, or behavior which you have experienced in the project, please
contact us at private@solr.apache.org .

## Security Vulnerability Reporting

If you believe you have identified a security vulnerability in this project, please send email to the ASF security
team at security@solr.apache.org, detailing the suspected issue and any methods you've found to reproduce it. More details
can be found [here](https://www.apache.org/security/)

Please do NOT open an issue in the GitHub repository, as we'd prefer to keep vulnerability reports private until
we've had an opportunity to review and address them.

## Acknowledgements

The Solr Operator was donated to Apache Solr by Bloomberg, after the v0.2.8 release.
Many thanks to their contributions over the years!
