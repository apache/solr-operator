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

# Solr Backups

The Solr Operator supports triggering the backup of arbitrary Solr collections.

Triggering these backups involves setting configuration options on both the SolrCloud and SolrBackup CRDs.
The SolrCloud instance is responsible for defining one or more backup "repositories" (metadata describing where and how the backup data should be stored).
SolrBackup instances then trigger backups by referencing these repositories by name, listing the Solr collections to back up, and optionally requesting some limited post-processing of the backup data (compression, relocation, etc).

For detailed information on how to best configure backups for your use case, please refer to the detailed schema information provided by `kubectl explain solrcloud.spec.dataStorage.backupRestoreOptions` and its child elements, as well as `kubectl explain solrbackup`.

This page outlines how to create and delete a Kubernetes SolrBackup

- [Creation](#creating-an-example-solrbackup)
- [Deletion](#deleting-an-example-solrbackup)
- [Repository Types](#supported-repository-types)
  - [GCS](#gcs-backup-repositories)
  - [S3](#s3-backup-repositories)

## Creating an example SolrBackup

A prerequisite for taking a backup is having something to take a backup _of_.
SolrCloud creation generally is covered in more detail [here](../solr-cloud/README.md), so if you don't have one already, create a SolrCloud instance as per those instructions.

Now that you have a Solr cluster to backup data from, you need a place to store the backup data.
In this example, we'll create a Kubernetes persistent volume to mount on each Solr node.

A volume for this purpose can be created as below:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: collection-backup-pvc
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Gi 
  storageClassName: hostpath
  volumeMode: Filesystem
```

Note that this PVC specifies `ReadWriteMany` access, which is required for Solr clusters with more than node.
Note also that it uses a `storageClassName` of `hostpath`.
Not all Kubernetes clusters support this `storageClassName` value - you may need to choose a different `ReadWriteMany`-compatible storage class based on your Kubernetes version and cluster setup.

Next, modify your existing SolrCloud instance by adding a backup repository definition that uses the recently created volume.
To do this, run `kubectl edit solrcloud example`, adding the following YAML nested under the `spec` property:

```yaml
spec:
  backupRepositories:
    - name: "local-collection-backups-1"
      managed:
        volume:
          persistentVolumeClaim:
            claimName: "collection-backup-pvc"
```

This defines a backup repository called "local-collection-backups-1" which is setup to store backup data on the volume we've just created.
The operator will notice this change and create new Solr pods that have the 'collection-backup-pvc' volume mounted.

Now that there's a backup repository available to use, a backup can be triggered anytime by creating a new SolrBackup instance.

```yaml
apiVersion: solr.apache.org/v1beta1
kind: SolrBackup
metadata:
  name: local-backup
  namespace: default
spec:
  repositoryName: "local-collection-backups-1"
  solrCloud: example
  collections:
    - techproducts
    - books
```

This will create a backup of both the 'techproducts' and 'books' collections, storing the data on the 'collection-backup-pvc' volume.
The status of our triggered backup can be checked with the command below.

```bash
$ kubectl get solrbackups
NAME                               CLOUD     FINISHED   SUCCESSFUL   AGE
local-backup   example   true       true         72s
```

## Deleting an example SolrBackup

Once the operator completes a backup, the SolrBackup instance can be safely deleted.

```bash
$ kubectl delete solrbackup local-backup
TODO command output
```

Note that deleting SolrBackup instances doesn't delete the backed up data, which the operator views as already persisted and outside its control.
In our example this data can still be found on the volume we created earlier

```bash
$ kubectl exec example-solrcloud-0 -- ls -lh /var/solr/data/backup-restore-managed-local-collection-backups-1/backups/
total 8K
drwxr-xr-x 3 solr solr 4.0K Sep 16 11:48 local-backup-books
drwxr-xr-x 3 solr solr 4.0K Sep 16 11:48 local-backup-techproducts
```

Managed backup data, as in our example, can always be deleted using standard shell commands if desired:

```bash
kubectl exec example-solrcloud-0 -- rm -r /var/solr/data/backup-restore/local-collection-backups-1/backups/local-backup-books
kubectl exec example-solrcloud-0 -- rm -r /var/solr/data/backup-restore/local-collection-backups-1/backups/local-backup-techproducts
```

## Supported Repository Types
_Since v0.5.0_

Note all repositories are defined in the `SolrCloud` specification.
In order to use a repository in the `SolrBackup` CRD, it must be defined in the `SolrCloud` spec.
All yaml examples below are `SolrCloud` resources, not `SolrBackup` resources.

The Solr-operator currently supports three different backup repository types: Google Cloud Storage ("GCS"), AWS S3 ("S3"), and managed ("local").
The cloud backup solutions (GCS and S3) are strongly suggested over the managed option, though they require newer Solr releases.

Multiple repositories can be defined under the `SolrCloud.spec.backupRepositories` field.
Specify a unique name and repo type that you want to connect to.
Repository-type specific options are found under the object named with the repository-type.
Examples can be found below under each repository-type section below.
Feel free to mix and match multiple backup repository types to fit your use case:

```yaml
spec:
  backupRepositories:
    - name: "local-collection-backups-1"
      managed:
        ...
    - name: "gcs-collection-backups-1"
      gcs:
        ...
```

### GCS Backup Repositories
_Since v0.5.0_

GCS Repositories store backup data remotely in Google Cloud Storage.
This repository type is only supported in deployments that use a Solr version >= `8.9.0`.

Each repository must specify the GCS bucket to store data in (the `bucket` property), and the name of a Kubernetes secret containing credentials for accessing GCS (the `gcsCredentialSecret` property).
This secret must have a key `service-account-key.json` whose value is a JSON service account key as described [here](https://cloud.google.com/iam/docs/creating-managing-service-account-keys)
If you already have your service account key, this secret can be created using a command like the one below.

```bash
kubectl create secret generic <secretName> --from-file=service-account-key.json=<path-to-service-account-key>
```

An example of a SolrCloud spec with only one backup repository, with type GCS:

```yaml
spec:
  backupRepositories:
    - name: "gcs-backups-1"
      gcs:
        bucket: "backup-bucket" # Required
        gcsCredentialSecret: # Required
          name: "secretName"
          key: "service-account-key.json"
        baseLocation: "/store/here" # Optional
```


### S3 Backup Repositories
_Since v0.5.0_

S3 Repositories store backup data remotely in AWS S3 (or a supported S3 compatible interface).
This repository type is only supported in deployments that use a Solr version >= `8.10.0`.

Each repository must specify an S3 bucket and region to store data in (the `bucket` and `region` properties).
Users will want to setup credentials so that the SolrCloud can connect to the S3 bucket and region, more information can be found in the [credentials section](#s3-credentials).

```yaml
spec:
  backupRepositories:
    - name: "s3-backups-1"
      s3:
        region: "us-west-2" # Required
        bucket: "backup-bucket" # Required
        credentials: {} # Optional
        proxyUrl: "https://proxy-url-for-s3:3242" # Optional
        endpoint: "https://custom-s3-endpoint:3242" # Optional
```

Users can also optionally set a `proxyUrl` or `endpoint` for the S3Repository.
More information on these settings can be found in the [Ref Guide](https://solr.apache.org/guide/8_10/making-and-restoring-backups.html#s3backuprepository).

#### S3 Credentials

The Solr `S3Repository` module uses the [default credential chain for AWS](https://docs.aws.amazon.com/sdk-for-java/v2/developer-guide/credentials.html#credentials-chain).
All of the options below are designed to be utilized by this credential chain.

There are a few options for giving a SolrCloud the credentials for connecting to S3.
The two most straightforward ways can be used via the `spec.backupRepositories.s3.credentials` property.

```yaml
spec:
  backupRepositories:
    - name: "s3-backups-1"
      s3:
        region: "us-west-2"
        bucket: "backup-bucket"
        credentials:
          accessKeyIdSecret: # Optional
            name: aws-secrets
            key: access-key-id
          secretAccessKeySecret: # Optional
            name: aws-secrets
            key: secret-access-key
          sessionTokenSecret: # Optional
            name: aws-secrets
            key: session-token
          credentialsFileSecret: # Optional
            name: aws-credentials
            key: credentials
```

All options in the `credentials` property are optional, as users can pick and choose which ones to use.
If you have all of your credentials setup in an [AWS Credentials File](https://docs.aws.amazon.com/sdkref/latest/guide/file-format.html#file-format-creds),
then `credentialsFileSecret` will be the only property you need to set.
However, if you don't have a credentials file, you will likely need to set at least the `accessKeyIdSecret` and `secretAccessKeySecret` properties.
All of these options require the referenced Kuberentes secrets to already exist before creating the SolrCloud resource.
_(If desired, all options can be combined. e.g. Use `accessKeyIdSecret` and `credentialsFileSecret` together. The ordering of the default credentials chain will determine which options are used.)_

The options in the credentials file above merely set environment variables on the pod, or in the case of `credentialsFileSecret` use an environment variable and a volume mount.
Users can decide to not use the `credentials` section of the s3 repository config, and instead set these environment variables themselves via `spec.customSolrKubeOptions.podOptions.env`.

Lastly, if running in EKS, it is possible to add [IAM information to Kubernetes serviceAccounts](https://aws.amazon.com/blogs/opensource/introducing-fine-grained-iam-roles-service-accounts/).
If this is done correctly, you will only need to specify the serviceAccount for the SolrCloud pods via `spec.customSolrKubeOptions.podOptions.serviceAccount`.

_NOTE: Because the Solr S3 Repository is using system-wide settings for AWS credentials, you cannot specify different credentials for different S3 repositories.
This may be addressed in future Solr versions, but for now use the same credentials for all s3 repos._

### Managed ("Local") Backup Repositories
_Since v0.5.0_

Managed repositories store backup data "locally" on a Kubernetes volume mounted by each Solr pod.
An example of a SolrCloud spec with only one backup repository, with type Managed:

```yaml
spec:
  backupRepositories:
    - name: "local-collection-backups-1"
      managed:
        volume: # Required
          persistentVolumeClaim:
            claimName: "collection-backup-pvc"
        directory: "store/here" # Optional
```
