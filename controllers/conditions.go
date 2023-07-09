package controllers

import (
	"context"
	trustyaiopendatahubiov1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	trustyAIAvailableConditionType   = "TrustyAIAvailable"
	pvcAvailableConditionType        = "PVCAvailable"
	modelMeshConfiguredConditionType = "ModelMeshConfigured"
)

// setCondition sets the status condition of the CR, guaranteeing that there are no duplicates
func (r *TrustyAIServiceReconciler) setCondition(instance *trustyaiopendatahubiov1alpha1.TrustyAIService, condition trustyaiopendatahubiov1alpha1.Condition) (bool, error) {
	condition.LastTransitionTime = metav1.Now()

	for i, c := range instance.Status.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status || c.Reason != condition.Reason || c.Message != condition.Message {
				instance.Status.Conditions[i] = condition
				return true, nil // A condition was modified
			}
			return false, nil // No changes were made
		}
	}

	instance.Status.Conditions = append(instance.Status.Conditions, condition)
	return true, nil // A condition was added
}

// updateCondition updates the status condition of the CR
func (r *TrustyAIServiceReconciler) updateCondition(ctx context.Context, req ctrl.Request, instance *trustyaiopendatahubiov1alpha1.TrustyAIService, conditionType string, status corev1.ConditionStatus, reason string, message string) error {
	condition := trustyaiopendatahubiov1alpha1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	changed, err := r.setCondition(instance, condition)
	if err != nil {
		return err
	}
	if changed {
		if err := r.Status().Update(ctx, instance); err != nil {
			log.FromContext(ctx).Error(err, "Failed to update TrustyAIService status")
			return err
		}
	}
	return nil
}

// allPodsRunning returns true if all Pods in the CR's deployment are running, otherwise false
func (r *TrustyAIServiceReconciler) allPodsRunning(ctx context.Context, req ctrl.Request, instance *trustyaiopendatahubiov1alpha1.TrustyAIService) (bool, error) {
	// Get Deployment's Pods status
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, req.NamespacedName, deployment)
	if err != nil {
		log.FromContext(ctx).Error(err, "Could not fetch deployment.")
		return false, err
	}

	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels(deployment.Spec.Selector.MatchLabels),
	}

	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.FromContext(ctx).Error(err, "Could not list pods.")
		return false, err
	}

	allPodsRunning := true
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			allPodsRunning = false
			break
		}
	}
	return allPodsRunning, nil
}

func (r *TrustyAIServiceReconciler) reconcileStatuses(instance *trustyaiopendatahubiov1alpha1.TrustyAIService, ctx context.Context) error {
	// Update the status of the custom resource
	err := r.Status().Update(ctx, instance)
	if err != nil {
		log.FromContext(ctx).Error(err, "Error updating conditions.")
		return err
	}
	return nil
}
