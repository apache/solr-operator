/*
Copyright 2019 Bloomberg Finance LP.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apis

import (
	solr "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	etcdop "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	zkop "github.com/pravega/zookeeper-operator/pkg/apis"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, solr.SchemeBuilder.AddToScheme)

	// Since solr-operator uses etcd-operator and zk-operator
	AddToSchemes = append(AddToSchemes, etcdop.AddToScheme)
	AddToSchemes = append(AddToSchemes, zkop.AddToScheme)
}
