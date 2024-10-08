package controllers

import (
	"context"
	"strings"

	trustyaiopendatahubiov1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *TrustyAIServiceReconciler) checkDatabaseAccessible(ctx context.Context, instance *trustyaiopendatahubiov1alpha1.TrustyAIService) (string, error) {
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return StatusDBConnecting, nil
		}
		return StatusDBConnectionError, err
	}

	for _, cond := range deployment.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
			podList := &corev1.PodList{}
			listOpts := []client.ListOption{
				client.InNamespace(instance.Namespace),
				client.MatchingLabels(deployment.Spec.Selector.MatchLabels),
			}
			if err := r.List(ctx, podList, listOpts...); err != nil {
				return StatusDBConnectionError, err
			}

			for _, pod := range podList.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.Name == "trustyai-service" {
						if cs.State.Running != nil {
							// Return DBConnecting while the container is running but no confirmation of a DB connection yet
							return StatusDBConnecting, nil
						}

						if cs.LastTerminationState.Terminated != nil {
							termination := cs.LastTerminationState.Terminated
							if termination.Reason == "Error" && termination.Message != "" {
								if strings.Contains(termination.Message, "Socket fail to connect to host:address") || strings.Contains(termination.Message, "Connection refused") {
									return StatusDBConnectionError, nil
								}
							}
						}

						if cs.State.Waiting != nil && cs.State.Waiting.Reason == StateReasonCrashLoopBackOff {
							return StatusDBConnectionError, nil
						}
					}
				}
			}
		}
	}

	return StatusDBConnecting, nil
}
