package util

import (
	"bytes"
	"fmt"
	solr "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"net/url"
)


const (
	BaseBackupRestorePath = "/var/solr-backup-restore"

	JobTTLSeconds = int32(60)
)

func BackupRestoreSubPathForCloud(cloud string) string {
	return "cloud/" + cloud
}

func BackupSubPathForCloud(cloud string, backupName string) string {
	return BackupRestoreSubPathForCloud(cloud) + "/backups/" + backupName
}

func RestoreSubPathForCloud(cloud string, restoreName string) string {
	return BackupRestoreSubPathForCloud(cloud) + "/restores/" + restoreName
}

func BackupPath(backupName string) string {
	return BaseBackupRestorePath +  "/backups/" + backupName
}

func RestorePath(backupName string) string {
	return BaseBackupRestorePath +  "/restores/" + backupName
}

func AsyncIdForCollectionBackup(collection string, backupName string) string {
	return backupName + "-" + collection
}

func CheckStatusOfCollectionBackups(backup *solr.SolrBackup) (allFinished bool) {
	fals := false

	// Check if all collection backups have been completed, this is updated in the loop
	allFinished = len(backup.Status.CollectionBackupStatuses) > 0

	// Check if persistence should be skipped if no backup completed successfully
	anySuccessful := false

	for _, collectionStatus := range backup.Status.CollectionBackupStatuses {
		allFinished = allFinished && collectionStatus.Finished
		anySuccessful = anySuccessful || (collectionStatus.Successful != nil && *collectionStatus.Successful)
	}
	if allFinished && !anySuccessful {
		backup.Status.Finished = true
		if backup.Status.Successful == nil {
			backup.Status.Successful = &fals
		}
	}
	return
}

func GenerateBackupPersistenceJobForCloud(backup *solr.SolrBackup, solrCloud *solr.SolrCloud) *batchv1.Job {
	volumeSource := corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: solrCloud.Spec.BackupRestorePvcName,
		},
	}

	return GenerateBackupPersistenceJob(backup, volumeSource, BackupSubPathForCloud(solrCloud.Name, backup.Name))
}

// GenerateBackupPersistenceJob creates a Job that will persist backup data and purge the backup from the solrBackupVolume
func GenerateBackupPersistenceJob(solrBackup *solr.SolrBackup, solrBackupVolume corev1.VolumeSource, backupSubPath string) *batchv1.Job {
	copyLabels := solrBackup.GetLabels()
	if copyLabels == nil {
		copyLabels = map[string]string{}
	}
	labels := solrBackup.SharedLabelsWith(solrBackup.GetLabels())

	//ttlSeconds := JobTTLSeconds

	image, env, command, volume, volumeMount, numRetries := GeneratePersistenceOptions(solrBackup)

	volumes := []corev1.Volume{
		{
			Name:         "backup-data",
			VolumeSource: solrBackupVolume,
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			MountPath: BaseBackupRestorePath,
			Name:      "backup-data",
			SubPath:   backupSubPath,
			ReadOnly:  false,
		},
	}
	if volume != nil && volumeMount != nil {
		volumes = append(volumes, *volume)
		volumeMounts = append(volumeMounts, *volumeMount)
	}

	parallelismAndCompletions := int32(1)


	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      solrBackup.PersistenceJobName(),
			Namespace: solrBackup.GetNamespace(),
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			//TTLSecondsAfterFinished: &ttlSeconds,
			BackoffLimit:            numRetries,
			Parallelism: &parallelismAndCompletions,
			Completions: &parallelismAndCompletions,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Volumes: volumes,
					Containers: [] corev1.Container{
						{
							Name:            "backup-persistence",
							Image:           image.ToImageName(),
							ImagePullPolicy: image.PullPolicy,
							VolumeMounts: volumeMounts,
							Env: env,
							Command: command,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	return job
}

// GeneratePersistenceOptions creates options for a Job that will persist backup data
func GeneratePersistenceOptions(solrBackup *solr.SolrBackup) (image solr.ContainerImage, envVars []corev1.EnvVar, command []string, volume *corev1.Volume, volumeMount *corev1.VolumeMount, numRetries *int32) {
	persistenceSource := solrBackup.Spec.Persistence
	if persistenceSource.Volume != nil {
		// Options for persisting to a volume
		image = persistenceSource.Volume.BusyBoxImage
		envVars = make([]corev1.EnvVar, 0)
		// Copy the information to the persistent storage, and delete it from the backup-restore volume.
		command = []string{"sh", "-c", "cp -r " + BaseBackupRestorePath + "/. /var/backup-persistence/ && chmod -R a+rwx /var/backup-persistence/* && rm -rf " + BaseBackupRestorePath + "/{*,.*}"}
		volume = &corev1.Volume{
			Name: "persistence",
			VolumeSource: persistenceSource.Volume.VolumeSource,
		}
		volumeMount = &corev1.VolumeMount{
			Name: "persistence",
			SubPath: persistenceSource.Volume.Path,
			ReadOnly: false,
			MountPath: "/var/backup-persistence",
		}
		r := int32(1)
		numRetries = &r
	} else if persistenceSource.S3 != nil {
		// Options for persisting to S3
		image = persistenceSource.S3.AWSCliImage
		envVars = make([]corev1.EnvVar, 0)
		command = []string{
			"mv", BaseBackupRestorePath + "/*",
		}
		numRetries = persistenceSource.S3.Retries
	}

	return image, envVars, command, volume, volumeMount, numRetries
}


func StartBackupForCollection(cloud string, collection string, backupName string, namespace string) (success bool, err error) {
	queryParams := url.Values{}
	queryParams.Add("action", "BACKUP")
	queryParams.Add("collection", collection)
	queryParams.Add("name", collection)
	queryParams.Add("location", BackupPath(backupName))
	queryParams.Add("async", AsyncIdForCollectionBackup(collection, backupName))

	resp := &SolrAsyncResponse{}

	log.Info("Calling to start collection backup", "namespace", namespace, "cloud", cloud, "collection", collection, "backup", backupName)
	err = CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		if resp.ResponseHeader.Status == 0 {
			success = true
		}
	} else {
		log.Error(err, "Error starting collection backup", "namespace", namespace, "cloud", cloud, "collection", collection, "backup", backupName)
	}

	return success, err
}


func CheckBackupForCollection(cloud string, collection string, backupName string, namespace string) (finished bool, success bool, asyncStatus string, err error) {
	queryParams := url.Values{}
	queryParams.Add("action", "REQUESTSTATUS")
	queryParams.Add("requestid", AsyncIdForCollectionBackup(collection, backupName))

	resp := &SolrAsyncResponse{}

	log.Info("Calling to check on collection backup", "namespace", namespace, "cloud", cloud, "collection", collection, "backup", backupName)
	err = CallCollectionsApi(cloud, namespace, queryParams, resp)

	if err == nil {
		if resp.ResponseHeader.Status == 0 {
			asyncStatus = resp.Status.AsyncState
			if resp.Status.AsyncState == "completed" {
				finished = true
				success = true
			}
			if resp.Status.AsyncState == "failed" {
				finished = true
				success = false
			}
		}
	} else {
		log.Error(err, "Error checking on collection backup", "namespace", namespace, "cloud", cloud, "collection", collection, "backup", backupName)
	}

	return finished, success, asyncStatus, err
}


func DeleteAsyncInfoForBackup(cloud string, collection string, backupName string, namespace string) (err error) {
	queryParams := url.Values{}
	queryParams.Add("action", "DELETESTATUS")
	queryParams.Add("requestid", AsyncIdForCollectionBackup(collection, backupName))

	resp := &SolrAsyncResponse{}

	log.Info("Calling to delete async info for backup command.", "namespace", namespace, "cloud", cloud, "collection", collection, "backup", backupName)
	err = CallCollectionsApi(cloud, namespace, queryParams, resp)
	if err != nil {
		log.Error(err, "Error deleting async data for collection backup", "namespace", namespace, "cloud", cloud, "collection", collection, "backup", backupName)
	}

	return err
}

type SolrAsyncResponse struct {
	ResponseHeader SolrResponseHeader `json:"responseHeader"`

	// +optional
	RequestId string `json:"requestId"`

	// +optional
	Status SolrAsyncStatus `json:"status"`
}

type SolrResponseHeader struct {
	Status int `json:"status"`

	QTime int `json:"QTime"`
}

type SolrAsyncStatus struct {
	// Possible states can be found here: https://github.com/apache/lucene-solr/blob/1d85cd783863f75cea133fb9c452302214165a4d/solr/solrj/src/java/org/apache/solr/client/solrj/response/RequestStatusState.java
	AsyncState string `json:"state"`

	Message string `json:"msg"`
}


func EnsureDirectoryForBackup(solrCloud *solr.SolrCloud, backup string, config *rest.Config) (err error) {
	backupPath := BackupPath(backup)
	// Create an empty directory for the backup
	return RunExecForPod(
		solrCloud.GetAllSolrNodeNames()[0],
		solrCloud.Namespace,
		[]string{"/bin/bash", "-c", "rm -rf " + backupPath + " && mkdir -p " + backupPath},
		*config,
	)
}

func RunExecForPod(podName string, namespace string, command []string, config rest.Config) (err error) {
	client := &kubernetes.Clientset{}
	if client, err = kubernetes.NewForConfig(&config); err != nil {
		return err
	}
	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   command,
		Container: "solrcloud-node",
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(&config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error while creating Executor: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return fmt.Errorf("error in Stream: %v", err)
	}

	return nil
}