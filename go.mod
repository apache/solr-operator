module github.com/apache/solr-operator

go 1.16

require (
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/elazarl/goproxy v0.0.0-20191011121108-aa519ddbe484 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-logr/logr v0.4.0
	github.com/go-logr/zapr v0.2.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/onsi/gomega v1.14.0
	github.com/pravega/zookeeper-operator v0.2.12
	github.com/robfig/cron/v3 v3.0.1
	github.com/stretchr/testify v1.6.1
	golang.org/x/net v0.0.0-20210716203947-853a461950ff
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/utils v0.0.0-20210709001253-0e1f9d693477
	sigs.k8s.io/controller-runtime v0.6.5
)

replace k8s.io/client-go => k8s.io/client-go v0.20.4
