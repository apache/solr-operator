module github.com/apache/solr-operator

go 1.16

require (
	github.com/coreos/go-oidc/v3 v3.1.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-logr/logr v0.3.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/shaj13/libcache v1.0.0
	github.com/stretchr/testify v1.7.0
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	google.golang.org/appengine v1.6.7 // indirect
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/controller-runtime v0.8.3
)
