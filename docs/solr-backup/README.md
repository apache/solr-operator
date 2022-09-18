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
SolrBackup instances then trigger backups by referencing these repositories by name, listing the Solr collections to back up, and optionally scheduling recurring backups.

For detailed information on how to best configure backups for your use case, please refer to the detailed schema information provided by `kubectl explain solrcloud.spec.backupRepositories` and its child elements, as well as `kubectl explain solrbackup`.

This page outlines how to create and delete a Kubernetes SolrBackup.

- [Creation](#creating-an-example-solrbackup)
- [Recurring/Scheduled Backups](#recurring-backups)
- [Deletion](#deleting-an-example-solrbackup)
- [Repository Types](#supported-repository-types)
  - [GCS](#gcs-backup-repositories)
  - [S3](#s3-backup-repositories)
  - [Volume](#volume-backup-repositories)

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
      volume:
        source:
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
NAME   CLOUD     STARTED   FINISHED   SUCCESSFUL   NEXTBACKUP  AGE
test   example   123m      true       false                     161m
```

## Recurring Backups
_Since v0.5.0_

The Solr Operator enables taking recurring updates, at a set interval.
Note that this feature requires a SolrCloud running Solr >= `8.9.0`, because it relies on `Incremental` backups.

By default the Solr Operator will save a maximum of **5** backups at a time, however users can override this using `SolrBackup.spec.recurrence.maxSaved`.
When using `recurrence`, users must provide a Cron-style `schedule` for the interval at which backups should be taken.
Please refer to the [GoLang cron-spec](https://pkg.go.dev/github.com/robfig/cron/v3?utm_source=godoc#hdr-CRON_Expression_Format) for more information on allowed syntax.

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
  recurrence: # Store one backup daily, and keep a week at a time.
    schedule: "@daily"
    maxSaved: 7
```

If using `kubectl`, the standard `get` command will return the time the backup was last started and when the next backup will occur.

```bash
$ kubectl get solrbackups
NAME   CLOUD     STARTED   FINISHED   SUCCESSFUL   NEXTBACKUP             AGE
test   example   123m      true       true         2021-11-09T00:00:00Z   161m
```

Much like when not taking a recurring backup, `SolrBackup.status` will contain the information from the latest, or currently running, backup.
The results of previous backup attempts are stored under `SolrBackup.status.history` (sorted from most recent to oldest).

You are able to **add or remove** `recurrence` to/from an existing `SolrBackup` object, no matter what stage that `SolrBackup` object is in.
If you add recurrence, then a new backup will be scheduled based on the `startTimestamp` of the last backup.
If you remove recurrence, then the `nextBackupTime` will be removed.
However, if the recurrent backup is already underway, it will not be stopped.

### Backup Scheduling

Backups are scheduled based on the `startTimestamp` of the last backup.
Therefore, if an interval schedule such as `@every 1h` is used, and a backup starts on `2021-11-09T03:10:00Z` and ends on `2021-11-09T05:30:00Z`, then the next backup will be started at `2021-11-09T04:10:00Z`.
If the interval is shorter than the time it takes to complete a backup, then the next backup will started directly after the previous backup completes (even though it is delayed from its given schedule).
And the next backup will be scheduled based on the `startTimestamp` of the delayed backup.
So there is a possibility of skew overtime if backups take longer than the allotted schedule.

If a guaranteed schedule is important, it is recommended to use intervals that are guaranteed to be longer than the time it takes to complete a backup.

### Temporarily Disabling Recurring Backups

It is also easy to temporarily disable backups for a time.
Merely add `disabled: true` under the `recurrence` section of the `SolrBackup` resource.
And set `disabled: false`, or just remove the property to re-enable backups.

Since backups are scheduled based on the `startTimestamp` of the last backup, a new backup may start immediately after you re-enable the recurrence.

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
  recurrence: # Store one backup daily, and keep a week at a time.
    schedule: "@daily"
    maxSaved: 7
    disabled: true
```

**Note: this will not stop any backups running at the time that `disabled: true` is set, it will only affect scheduling future backups.**

## Deleting an example SolrBackup

Once the operator completes a backup, the SolrBackup instance can be safely deleted.

```bash
$ kubectl delete solrbackup local-backup
```

Note that deleting SolrBackup instances doesn't delete the backed up data, which the operator views as already persisted and outside its control.
In our example this data can still be found on the volume we created earlier

```bash
$ kubectl exec example-solrcloud-0 -- ls -lh /var/solr/data/backup-restore/local-collection-backups-1/backups/
total 8K
drwxr-xr-x 3 solr solr 4.0K Sep 16 11:48 local-backup-books
drwxr-xr-x 3 solr solr 4.0K Sep 16 11:48 local-backup-techproducts
```

Volume backup data, as in our example, can always be deleted using standard shell commands if desired:

```bash
kubectl exec example-solrcloud-0 -- rm -r /var/solr/data/backup-restore/local-collection-backups-1/backups/local-backup-books
kubectl exec example-solrcloud-0 -- rm -r /var/solr/data/backup-restore/local-collection-backups-1/backups/local-backup-techproducts
```

## Supported Repository Types
_Since v0.5.0_

Note all repositories are defined in the `SolrCloud` specification.
In order to use a repository in the `SolrBackup` CRD, it must be defined in the `SolrCloud` spec.
All yaml examples below are `SolrCloud` resources, not `SolrBackup` resources.

The Solr-operator currently supports three different backup repository types: Google Cloud Storage ("GCS"), AWS S3 ("S3"), and Volume ("local").
The cloud backup solutions (GCS and S3) are strongly suggested as they are cloud-native backup solutions, however they require newer Solr versions.

Multiple repositories can be defined under the `SolrCloud.spec.backupRepositories` field.
Specify a unique name and single repo type that you want to connect to.
Repository-type specific options are found under the object named with the repository-type.
Examples can be found below under each repository-type section below.
Feel free to mix and match multiple backup repository types to fit your use case (or multiple repositories of the same type):

```yaml
spec:
  backupRepositories:
    - name: "local-collection-backups-1"
      volume:
        ...
    - name: "gcs-collection-backups-1"
      gcs:
        ...
    - name: "s3-collection-backups-1"
      s3:
        ...
    - name: "s3-collection-backups-2"
      s3:
        ...
```

### GCS Backup Repositories
_Since v0.5.0_

GCS Repositories store backup data remotely in Google Cloud Storage.
This repository type is only supported in deployments that use a Solr version >= `8.9.0`.

Each repository must specify the GCS bucket to store data in (the `bucket` property), and (usually) the name of a Kubernetes secret containing credentials for accessing GCS (the `gcsCredentialSecret` property).
This secret must have a key `service-account-key.json` whose value is a JSON service account key as described [here](https://cloud.google.com/iam/docs/creating-managing-service-account-keys)
If you already have your service account key, this secret can be created using a command like the one below.

```bash
kubectl create secret generic <secretName> --from-file=service-account-key.json=<path-to-service-account-key>
```

In some rare cases (e.g. when deploying in GKE and relying on its [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) feature) explicit credentials are not required to talk to GCS.  In these cases, the `gcsCredentialSecret` property may be omitted.

An example of a SolrCloud spec with only one backup repository, with type GCS:

```yaml
spec:
  backupRepositories:
    - name: "gcs-backups-1"
      gcs:
        bucket: "backup-bucket" # Required
        gcsCredentialSecret: 
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

### Volume Backup Repositories
_Since v0.5.0_

Volume repositories store backup data "locally" on a Kubernetes volume mounted to each Solr pod.
An example of a SolrCloud spec with only one backup repository, with type Volume:

```yaml
spec:
  backupRepositories:
    - name: "local-collection-backups-1"
      volume:
        source: # Required
          persistentVolumeClaim:
            claimName: "collection-backup-pvc"
        directory: "store/here" # Optional
```

**NOTE: All persistent volumes used with Volume Repositories must have `accessMode: ReadWriteMany` set, otherwise the backups will not succeed.**
