package controllers

import (
	"context"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = FDescribe("SolrCloud controller - Node Roles", func() {
	var solrCloud *solrv1beta1.SolrCloud

	BeforeEach(func() {
		solrCloud = &solrv1beta1.SolrCloud{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: solrv1beta1.SolrCloudSpec{
				Replicas: pointer.Int32(3),
				ZookeeperRef: &solrv1beta1.ZookeeperRef{
					ConnectionInfo: &solrv1beta1.ZookeeperConnectionInfo{InternalConnectionString: "host:7271"},
				},
			},
		}
	})

	JustBeforeEach(func(ctx context.Context) {
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())
		expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
			g.Expect(found.WithDefaults(logger)).To(BeFalse())
		})
	})

	AfterEach(func(ctx context.Context) {
		cleanupTest(ctx, solrCloud)
	})

	FContext("No node roles", func() {
		FIt("only creates the data StatefulSet", func(ctx context.Context) {
			expectStatefulSet(ctx, solrCloud, solrCloud.StatefulSetName())
			expectNoStatefulSet(ctx, solrCloud, solrCloud.StatefulSetNameForRole(solrv1beta1.SolrNodeRoleOverseer))
			expectNoStatefulSet(ctx, solrCloud, solrCloud.StatefulSetNameForRole(solrv1beta1.SolrNodeRoleCoordinator))
			// We still want to ensure that there are 3 replicas for the data sts
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, sts *appsv1.StatefulSet) {
				g.Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
			})
		})
	})

	FContext("Overseers and coordinators", func() {
		BeforeEach(func() {
			solrCloud.Spec.NodeRoleDistribution = &solrv1beta1.NodeRoleDistribution{
				Overseers:    pointer.Int32(2),
				Coordinators: pointer.Int32(1),
			}
		})

		FIt("creates a StatefulSet per role", func(ctx context.Context) {
			// We still want to ensure that there are 3 replicas for the data sts
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, sts *appsv1.StatefulSet) {
				g.Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
			})
			overseerName := solrCloud.StatefulSetNameForRole(solrv1beta1.SolrNodeRoleOverseer)
			expectStatefulSetWithChecks(ctx, solrCloud, overseerName, func(g Gomega, sts *appsv1.StatefulSet) {
				g.Expect(*sts.Spec.Replicas).To(Equal(int32(2)))
				g.Expect(sts.Labels).To(HaveKeyWithValue(solrv1beta1.SolrNodeRoleLabel, string(solrv1beta1.SolrNodeRoleOverseer)))
				testPodEnvVariablesWithGomega(g, map[string]string{
					"SOLR_OPTS": "-DhostPort=$(SOLR_NODE_PORT) -Dsolr.node.roles=data:off,overseer:preferred,coordinator:off",
				}, sts.Spec.Template.Spec.Containers[0].Env)
			})
			coordinatorName := solrCloud.StatefulSetNameForRole(solrv1beta1.SolrNodeRoleCoordinator)
			expectStatefulSetWithChecks(ctx, solrCloud, coordinatorName, func(g Gomega, sts *appsv1.StatefulSet) {
				g.Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
				g.Expect(sts.Labels).To(HaveKeyWithValue(solrv1beta1.SolrNodeRoleLabel, string(solrv1beta1.SolrNodeRoleCoordinator)))
				//g.Expect(sts.Spec.Selector.MatchLabels).To(HaveKeyWithValue(coordinatorName, coordinatorName))
				testPodEnvVariablesWithGomega(g, map[string]string{
					"SOLR_OPTS": "-DhostPort=$(SOLR_NODE_PORT) -Dsolr.node.roles=data:off,overseer:disallowed,coordinator:on",
				}, sts.Spec.Template.Spec.Containers[0].Env)
			})
		})

		FIt("deletes a role's StatefulSet when the role is removed", func(ctx context.Context) {
			overseerName := solrCloud.StatefulSetNameForRole(solrv1beta1.SolrNodeRoleOverseer)
			coordinatorName := solrCloud.StatefulSetNameForRole(solrv1beta1.SolrNodeRoleCoordinator)
			expectStatefulSet(ctx, solrCloud, overseerName)
			expectStatefulSet(ctx, solrCloud, coordinatorName)

			expectSolrCloudWithChecks(ctx, solrCloud, func(g Gomega, found *solrv1beta1.SolrCloud) {
				found.Spec.NodeRoleDistribution = nil
				g.Expect(k8sClient.Update(ctx, found)).To(Succeed(), "Not here?")
			})
			eventuallyExpectNoStatefulSet(ctx, solrCloud, overseerName)
			eventuallyExpectNoStatefulSet(ctx, solrCloud, coordinatorName)
			// We still want to ensure that there are 3 replicas for the data sts
			expectStatefulSetWithChecks(ctx, solrCloud, solrCloud.StatefulSetName(), func(g Gomega, sts *appsv1.StatefulSet) {
				g.Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
			})
		})
	})
})
