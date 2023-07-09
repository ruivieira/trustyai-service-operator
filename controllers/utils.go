package controllers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

func isDeploymentReady(deployment *appsv1.Deployment) bool {
	return deployment.Status.Replicas == deployment.Status.UpdatedReplicas &&
		deployment.Status.Replicas == deployment.Status.AvailableReplicas
}

// containsString checks if a list contains a string
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// removeString removes a string from a list
func removeString(list []string, s string) []string {
	newList := []string{}
	for _, v := range list {
		if v != s {
			newList = append(newList, v)
		}
	}
	return newList
}

// GetNamespace returns the namespace of a pod
func GetNamespace() (string, error) {
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}
	return string(ns), nil
}

// GetDeploymentsByLabel returns a list of Deployments that match a label key-value pair
func (r *TrustyAIServiceReconciler) GetDeploymentsByLabel(ctx context.Context, namespace string, labelKey string, labelValue string) ([]appsv1.Deployment, error) {
	// Prepare a DeploymentList object
	deployments := &appsv1.DeploymentList{}

	// Define the selector based on the provided label key-value pair
	selector := labels.Set{labelKey: labelValue}.AsSelector()

	// Fetch the Deployments that match the selector
	if err := r.List(ctx, deployments, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		log.FromContext(ctx).Error(err, "Could not list Deployments by label.")
		return nil, err
	}

	return deployments.Items, nil
}

func DoNotRequeue() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func RequeueWithError(err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}

func RequeueAfterDuration(duration time.Duration) (ctrl.Result, error) {
	return ctrl.Result{RequeueAfter: duration}, nil
}

func RequeueAfterDurationWithError(duration time.Duration, err error) (ctrl.Result, error) {
	return ctrl.Result{RequeueAfter: duration}, err
}
