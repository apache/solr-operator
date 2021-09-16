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

## Creating an example SolrBackup

A prerequisite for taking a backup is having something to take a backup _of_.
SolrCloud creation generally is covered in more detail [here](../solr-cloud/README.md), so if you don't have one already, create a SolrCloud instance as per those instructions.

Now that you have a Solr cluster to backup data from, you need a place to store the backup data.
In this example, we'll create a Kubernetes persistent volume to mount on each Solr node.

A volume for this purpose can be created as below:

```
$ echo "
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
" > backup-pvc.yaml
$ cat backup-pvc.yaml | kubectl apply -f -
persistentvolumeclaim/collection-backup-pvc created
```

Note that this PVC specifies `ReadWriteMany` access, which is required for Solr clusters with more than node.
Note also that it uses a `storageClassName` of `hostpath`.
Not all Kubernetes clusters support this `storageClassName` value - you may need to choose a different `ReadWriteMany`-compatible storage class based on your Kubernetes version and cluster setup.

Next, modify your existing SolrCloud instance by adding a backup repository definition that uses the recently created volume.
To do this, run `kubectl edit solrcloud example`, adding the following YAML nested under the `spec` property:

```
dataStorage:
  backupRestoreOptions:
    managedRepositories:
      - name: "local-collection-backups-1"
        volume:
          persistentVolumeClaim:
            claimName: "collection-backup-pvc"
```

This defines a backup repository called "local-collection-backups-1" which is setup to store backup data on the volume we've just created.
The operator will notice this change and create new Solr pods that have the 'collection-backup-pvc' volume mounted.

Now that there's a backup repository available to use, a backup can be triggered anytime by creating a new SolrBackup instance.

```
$ echo "
apiVersion: solr.apache.org/v1beta1
kind: SolrBackup
metadata:
  name: local-backup-without-persistence
  namespace: default
spec:
  repositoryName: "local-collection-backups-1"
  solrCloud: example
  collections:
    - techproducts
    - books
" > backup-trigger.yaml
$ cat backup-trigger.yaml | kubectl apply -f
solrbackup.solr.apache.org/local-backup-without-persistence created
```

This will create a backup of both the 'techproducts' and 'books' collections, storing the data on the 'collection-backup-pvc' volume.
The status of our triggered backup can be checked with the command below.

```
$ kubectl get solrbackups
NAME                               CLOUD     FINISHED   SUCCESSFUL   AGE
local-backup-without-persistence   example   true       true         72s
```

## Deleting an example SolrBackup

Once the operator completes a backup, the SolrBackup instance can be safely deleted.

```
$ kubectl delete solrbackup local-backup-without-persistence
TODO command output
```

Note that deleting SolrBackup instances doesn't delete the backed up data, which the operator views as already persisted and outside its control.
In our example this data can still be found on the volume we created earlier

```
$ kubectl exec example-solrcloud-0 -- ls -lh /var/solr/data/backup-restore-managed-local-collection-backups-1/backups/local-backup-without-persistence
total 8K
drwxr-xr-x 3 solr solr 4.0K Sep 16 11:48 books
drwxr-xr-x 3 solr solr 4.0K Sep 16 11:48 techproducts
```

Managed backup data, as in our example, can always be deleted using standard shell commands if desired:

```
kubectl exec example-solrcloud-0 -- rm -r /var/solr/data/backup-restore-managed-local-collection-backups-1/backups/local-backup-without-persistence
```

## Supported Repository Types

The Solr-operator currently supports two different backup repository types: managed ("local") and Google Cloud Storage ("GCS")

### Managed ("Local") Backup Repositories

Managed repositories store backup data "locally" on a Kubernetes volume mounted by each Solr pod.
Managed repositories are so called because with the data stored in and managed by Kubernetes, the operator is able to offer a few advanced post-processing features that are unavailable for other repository types.

The main example of this currently is the operator's "persistence" feature, which upon completion of the backup will compress the backup files and optionally relocate the archive to a more permanent volume.  See [the example here](../../example/test_local_backup_with_persistence.yaml) for more details.

### GCS Backup Repositories

GCS Repositories store backup data remotely in Google Cloud Storage.
This repository type is only supported in deployments that use a Solr version >= 8.9.0.

Each repository must specify the GCS bucket to store data in (the `bucket` property), and the name of a Kubernetes secret containing credentials for accessing GCS (the `gcsCredentialSecret` property).
This secret must have a key `service-account-key.json` whose value is a JSON service account key as described [here](https://cloud.google.com/iam/docs/creating-managing-service-account-keys)
If you already have your service account key, this secret can be created using a command like the one below.

```
kubectl create secret generic <secretName> --from-file=service-account-key.json=<path-to-service-account-key>
```
