package util

func BackupReadyAnnotation() string {
	return "solrbackup.solr.bloomberg.com/ready"
}

func PathForBackup(backupName string) string {
	return "/backups/" + backupName
}
