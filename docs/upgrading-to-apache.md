# Upgrading the Solr Operator from Bloomberg to Apache

1. Install the v0.2.8 Solr Operator CRDs
2. Upgrade the current Solr Operator to v0.2.8
3. Install the Apache Solr Operator
    helm install apache apache-solr/solr-operator --version v0.3.0
4. fetch all BB resources, and upgrade them to Apache Resources
    - Should we make a script?
    - Definitely need to test this out
    


## Need to account for
- Upgrade of Zookeeper Operator dependency
- Location of helm chart