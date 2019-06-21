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

package solrcloud

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"testing"
	"time"

	solr "github.com/bloomberg/solr-operator/pkg/apis/solr/v1beta1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var (
	expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
	ssKey           = types.NamespacedName{Name: "foo-solrcloud", Namespace: "default"}
	csKey           = types.NamespacedName{Name: "foo-solrcloud-common", Namespace: "default"}
	hsKey           = types.NamespacedName{Name: "foo-solrcloud-headless", Namespace: "default"}
	iKey            = types.NamespacedName{Name: "foo-solrcloud-common", Namespace: "default"}
)

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	SetIngressBaseUrl("")
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Check the statefulSet
	expectStatefulSet(g, requests)

	// Check the client Service
	expectService(g, requests, csKey)

	// Check the headless Service
	expectService(g, requests, hsKey)

	// Check the ingress
	expectNoIngress(g, requests)
}

func TestReconcileWithIngress(t *testing.T) {
	SetIngressBaseUrl("ing.base.domain")
	UseEtcdCRD(false)
	UseZkCRD(true)
	g := gomega.NewGomegaWithT(t)
	instance := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			ZookeeperRef: &solr.ZookeeperRef{
				ConnectionInfo: &solr.ZookeeperConnectionInfo{
					InternalConnectionString: "host:7271",
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the SolrCloud object and expect the Reconcile and StatefulSet to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Check the statefulSet
	expectStatefulSet(g, requests)

	// Check the client Service
	expectService(g, requests, csKey)

	// Check the headless Service
	expectService(g, requests, hsKey)

	// Check the ingress
	expectIngress(g, requests)
}

func expectStatefulSet(g *gomega.GomegaWithT, requests chan reconcile.Request) {
	stateful := &appsv1.StatefulSet{}
	g.Eventually(func() error { return c.Get(context.TODO(), ssKey, stateful) }, timeout).
		Should(gomega.Succeed())

	// Delete the StatefulSet and expect Reconcile to be called for StatefulSet deletion
	g.Expect(c.Delete(context.TODO(), stateful)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), ssKey, stateful) }, timeout).
		Should(gomega.Succeed())

	// Manually delete StatefulSet since GC isn't enabled in the test control plane
	g.Eventually(func() error { return c.Delete(context.TODO(), stateful) }, timeout).
		Should(gomega.MatchError("statefulsets.apps \"" + ssKey.Name + "\" not found"))
}

func expectService(g *gomega.GomegaWithT, requests chan reconcile.Request, serviceKey types.NamespacedName) {
	service := &corev1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), serviceKey, service) }, timeout).
		Should(gomega.Succeed())

	// Delete the Service and expect Reconcile to be called for Service deletion
	g.Expect(c.Delete(context.TODO(), service)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), serviceKey, service) }, timeout).
		Should(gomega.Succeed())

	// Manually delete Service since GC isn't enabled in the test control plane
	g.Eventually(func() error { return c.Delete(context.TODO(), service) }, timeout).
		Should(gomega.MatchError("services \"" + serviceKey.Name + "\" not found"))
}

func expectIngress(g *gomega.GomegaWithT, requests chan reconcile.Request) {
	ingress := &extv1.Ingress{}
	g.Eventually(func() error { return c.Get(context.TODO(), iKey, ingress) }, timeout).
		Should(gomega.Succeed())

	// Delete the Ingress and expect Reconcile to be called for Ingress deletion
	g.Expect(c.Delete(context.TODO(), ingress)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), iKey, ingress) }, timeout).
		Should(gomega.Succeed())

	// Manually delete Ingress since GC isn't enabled in the test control plane
	g.Eventually(func() error { return c.Delete(context.TODO(), ingress) }, timeout).
		Should(gomega.MatchError("ingresses.extensions \"" + iKey.Name + "\" not found"))
}

func expectNoIngress(g *gomega.GomegaWithT, requests chan reconcile.Request) {
	ingress := &extv1.Ingress{}
	g.Eventually(func() error { return c.Get(context.TODO(), iKey, ingress) }, timeout).
		Should(gomega.MatchError("Ingress.extensions \"" + iKey.Name + "\" not found"))
}
