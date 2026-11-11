package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = FDescribe("E2E - SolrCloud - Node Roles", func() {
	var (
		solrCloud *solrv1beta1.SolrCloud

		solrCollection1 = "e2e-1"
	)

	// We need it to be basic for our other test
	BeforeEach(func() {
		solrCloud = generateBaseSolrCloud(2)
	})

	JustBeforeEach(func(ctx context.Context) {
		By("creating the SolrCloud")
		Expect(k8sClient.Create(ctx, solrCloud)).To(Succeed())

		DeferCleanup(func(ctx context.Context) {
			cleanupTest(ctx, solrCloud)
		})

		By("Waiting for the SolrCloud to come up healthy")
		solrCloud = expectSolrCloudToBeReady(ctx, solrCloud)

		By("creating a first Solr Collection")
		createAndQueryCollection(ctx, solrCloud, solrCollection1, 1, 2)
	})

	FContext("Statefulsets for roles exist - Solr Behaves as expected", func() {
		BeforeEach(func() {
			solrCloud = generateBaseSolrCloudWithNodeRoles(2, 2, 1)
		})

		FIt("Ensure non-data node roles are ready", func(ctx context.Context) {
			expectNonDataStatefulSetsReady(ctx, solrCloud)
		})

		FIt("Ensure overseer failover works", func(ctx context.Context) {
			expectNonDataStatefulSetsReady(ctx, solrCloud)
			overseerNode := solrNodePattern(solrCloud, solrv1beta1.SolrNodeRoleOverseer)

			By("Checking an overseer node is the leader")
			var leader string
			Eventually(func(g Gomega, ctx context.Context) {
				leader = fetchOverseerLeader(ctx, g, solrCloud)
				g.Expect(leader).NotTo(BeEmpty())
				g.Expect(leader).To(MatchRegexp(overseerNode.String()))
			}).WithContext(ctx).Within(time.Minute).Should(Succeed(), "Unable to find overseer leader")

			By("Deleting the overseer leader pod")
			leaderPod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: podNameFromSolrNode(leader), Namespace: solrCloud.Namespace}, leaderPod)).To(Succeed())
			Expect(k8sClient.Delete(ctx, leaderPod)).To(Succeed())

			By("Checking the other overseer is now the leader")
			Eventually(func(g Gomega, ctx context.Context) {
				newLeader := fetchOverseerLeader(ctx, g, solrCloud)
				g.Expect(newLeader).NotTo(BeEmpty())
				g.Expect(newLeader).To(MatchRegexp(overseerNode.String()))
				g.Expect(newLeader).NotTo(Equal(leader))
			}).WithContext(ctx).Within(time.Minute*2).Should(Succeed(), "Overseer leadership did not move node")
		})

		FIt("Replicas are only placed on data nodes", func(ctx context.Context) {
			expectNonDataStatefulSetsReady(ctx, solrCloud)
			dataNode := solrNodePattern(solrCloud, solrv1beta1.SolrNodeRoleData)

			clusterStatus := solr_api.SolrClusterStatusResponse{}
			Expect(json.Unmarshal([]byte(fetchClusterStatus(ctx, solrCloud)), &clusterStatus)).To(Succeed())

			collection, ok := clusterStatus.ClusterStatus.Collections[solrCollection1]
			Expect(ok).To(BeTrue(), "Collection %s missing from CLUSTERSTATUS", solrCollection1)
			for shardName, shard := range collection.Shards {
				for replicaName, replica := range shard.Replicas {
					Expect(replica.NodeName).To(MatchRegexp(dataNode.String()), "Replica %s of shard %s was placed on non-data node", replicaName, shardName)
				}
			}
		})

		FIt("Serve queries through the coordinator", func(ctx context.Context) {
			expectNonDataStatefulSetsReady(ctx, solrCloud)
			coordinatorNode := solrNodePattern(solrCloud, solrv1beta1.SolrNodeRoleCoordinator)

			By("Checking the coordinator is valid")
			var coordinator string
			Eventually(func(g Gomega, ctx context.Context) {
				coordinator = fetchOverseerLeader(ctx, g, solrCloud)
				g.Expect(coordinator).NotTo(BeEmpty())
				g.Expect(coordinator).To(MatchRegexp(coordinatorNode.String()))
			}).WithContext(ctx).Within(time.Minute).WithPolling(time.Second*5).Should(Succeed(), "Unable to find overseer leader")

			Eventually(func(g Gomega, ctx context.Context) {
				response, err := callSolrApiInPod(ctx, solrCloud, "get", fmt.Sprintf("/solr/%s/select", solrCollection1),
					map[string]string{"rows": "0"}, coordinator)
				g.Expect(err).NotTo(HaveOccurred(), "Error while querying solr")
				g.Expect(response).To(ContainSubstring("\"numFound\":0"))
			}).WithContext(ctx).Within(time.Second * 30).Should(Succeed())
		})
	})

	FContext("Node roles added to and removed from a running SolrCloud", func() {
		FIt("Adding non-data roles and ensuring the data nodes don't get restarted", func(ctx context.Context) {
			By("Recording the UID of the data pods first")
			dataPodUIDS := map[string]types.UID{}
			for _, podName := range solrCloud.GetSolrPodNames(int(*solrCloud.Spec.Replicas)) {
				pod := &corev1.Pod{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: solrCloud.Namespace}, pod)).To(Succeed())
				dataPodUIDS[podName] = pod.UID
			}

			By("Adding additional Node Roles")
			originalSolrCloud := solrCloud.DeepCopy()
			solrCloud.Spec.NodeRoleDistribution = &solrv1beta1.NodeRoleDistribution{
				Overseers: pointer.Int32(2),
			}
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not update the solr cloud spec")
			expectNonDataStatefulSetsReady(ctx, solrCloud)

			By("Checking that the data pods did not restart")
			for podName, uid := range dataPodUIDS {
				pod := &corev1.Pod{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: solrCloud.Namespace}, pod)).To(Succeed())
				Expect(pod.UID).To(Equal(uid), "Data pod %s was recreated when a non-data node role was added", podName)
			}

			By("Removing additional Node Roles")
			originalSolrCloud = solrCloud.DeepCopy()
			solrCloud.Spec.NodeRoleDistribution = nil
			Expect(k8sClient.Patch(ctx, solrCloud, client.MergeFrom(originalSolrCloud))).To(Succeed(), "Could not update the solr cloud spec")
			for _, role := range []solrv1beta1.SolrNodeRole{
				solrv1beta1.SolrNodeRoleOverseer,
				solrv1beta1.SolrNodeRoleCoordinator,
			} {
				eventuallyExpectNoStatefulSet(ctx, solrCloud, solrCloud.StatefulSetNameForRole(role))
			}

			By("Checking again that the data pods did not restart")
			for podName, uid := range dataPodUIDS {
				pod := &corev1.Pod{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: solrCloud.Namespace}, pod)).To(Succeed())
				Expect(pod.UID).To(Equal(uid), "Data pod %s was recreated when a non-data node role was added", podName)
			}

			By("Checking that a data pod then become an overseer")
			dataNode := solrNodePattern(solrCloud, solrv1beta1.SolrNodeRoleData)
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(fetchOverseerLeader(ctx, g, solrCloud)).To(MatchRegexp(dataNode.String()))
			}).WithContext(ctx).Within(time.Minute*2).WithPolling(time.Second*5).Should(Succeed(), "No data node became an overseer")

			queryCollection(ctx, solrCloud, solrCollection1, 0)
		})
	})
})
