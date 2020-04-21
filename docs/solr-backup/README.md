# Solr Backups

Solr backups require 3 things:
- A solr cloud running in kubernetes to backup
- The list of collections to backup
- A shared volume reference that can be written to from many clouds
    - This could be a NFS volume, a persistent volume claim (that has `ReadWriteMany` access), etc.
    - The same volume can be used for many solr clouds in the same namespace, as the data stored within the volume is namespaced.
- A way to persist the data. The currently supported persistence methods are:
    - A volume reference (this does not have to be `ReadWriteMany`)
    - An S3 endpoint.
    
Backups will be tarred before they are persisted.

There is no current way to restore these backups, but that is in the roadmap to implement.
