/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

import (
	"context"
	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
)

// BalanceReplicasForCluster takes a SolrCloud and balances all replicas across the Pods that are currently alive.
//
// Note: unlike EvictReplicasForPodIfNecessary, the only way we know that we are done balancingReplicas is by seeing
// a successful status returned from the command. So if we delete the asyncStatus, and then something happens in the operator,
// and we lose our state, then we will need to retry the balanceReplicas command. This should be ok since calling
// balanceReplicas multiple times should not be bad when the replicas for the cluster are already balanced.
func BalanceReplicasForCluster(ctx context.Context, solrCloud *solr.SolrCloud, statefulSet *appsv1.StatefulSet, balanceReason string, balanceCmdUniqueId string, logger logr.Logger) (balanceComplete bool, err error) {
	logger = logger.WithValues("balanceReason", balanceReason)
	// If the Cloud has 1 or zero pods, there is no reason to balance replicas.
	if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas < 1 {
		balanceComplete = true
	} else {
		requestId := "balance-replicas-" + balanceCmdUniqueId

		// First check to see if the Async Balance request has started
		if asyncState, message, asyncErr := solr_api.CheckAsyncRequest(ctx, solrCloud, requestId); asyncErr != nil {
			err = asyncErr
			logger.Error(err, "Error occurred while checking the status of the balance replicas task. Will try again.", "requestId", requestId)
		} else if asyncState == "notfound" {
			// Only start the balance command if all pods are ready
			if *statefulSet.Spec.Replicas != statefulSet.Status.ReadyReplicas {
				logger.Info("Cannot start balancing replicas until all pods are ready.", "pods", *statefulSet.Spec.Replicas, "readyPods", statefulSet.Status.ReadyReplicas)
			} else {
				// Submit new BalanceReplicas request
				rebalanceRequest := &solr_api.SolrRebalanceRequest{
					WaitForFinalState: true,
					Async:             requestId,
				}
				rebalanceResponse := &solr_api.SolrAsyncResponse{}
				err = solr_api.CallCollectionsApiV2(ctx, solrCloud, "POST", "/api/cluster/rebalanceReplicas", nil, rebalanceRequest, rebalanceResponse)
				if isUnsupportedApi, apiError := solr_api.CheckForCollectionsApiError("BALANCE_REPLICAS", rebalanceResponse.ResponseHeader, rebalanceResponse.Error); isUnsupportedApi {
					// TODO: Remove this if-statement when Solr 9.3 is the lowest supported version
					logger.Error(err, "Could not balance replicas across the cluster, because the SolrCloud's version does not support this feature.")
					// Swallow the error after logging it, because it's not a real error.
					// Balancing is not supported, so we just need to finish the clusterOp.
					err = nil
					balanceComplete = true
				} else if apiError != nil {
					err = apiError
				}
				if err == nil {
					logger.Info("Started balancing replicas across cluster.", "requestId", requestId)
				} else {
					logger.Error(err, "Could not balance replicas across the cluster. Will try again.")
				}
			}
		} else {
			logger.Info("Found async status", "requestId", requestId, "state", asyncState)
			// Only continue to delete the pod if the ReplaceNode request is complete and successful
			if asyncState == "completed" {
				balanceComplete = true
				logger.Info("Replica Balancing command completed successfully")
			} else if asyncState == "failed" {
				logger.Info("Replica Balancing command failed. Will try again", "message", message)
			}

			// Delete the async request Id if the async request is successful or failed.
			// If the request failed, this will cause a retry since the next reconcile won't find the async requestId in Solr.
			if asyncState == "completed" || asyncState == "failed" {
				if _, err = solr_api.DeleteAsyncRequest(ctx, solrCloud, requestId); err != nil {
					logger.Error(err, "Could not delete Async request status.", "requestId", requestId)
					balanceComplete = false
				}
			}
		}
	}
	return
}
