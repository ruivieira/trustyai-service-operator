/*
Copyright 2023.

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

package controllers

import (
	"context"
	goerrors "errors"
	"fmt"
	kserveapi "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	trustyaiopendatahubiov1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

var ErrPVCNotReady = goerrors.New("PVC is not ready")

const (
	defaultImage         = string("quay.io/trustyai/trustyai-service")
	defaultTag           = string("latest")
	containerName        = "trustyai-service"
	serviceMonitorName   = "trustyai-metrics"
	finalizerName        = "trustyai.opendatahub.io.trustyai.opendatahub.io/finalizer"
	payloadProcessorName = "MM_PAYLOAD_PROCESSORS"
	modelMeshLabelKey    = "modelmesh-service"
	modelMeshLabelValue  = "modelmesh-serving"
	volumeMountName      = "volume"
)

// TrustyAIServiceReconciler reconciles a TrustyAIService object
type TrustyAIServiceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Namespace string
}

//+kubebuilder:rbac:groups=trustyai.opendatahub.io.trustyai.opendatahub.io,resources=trustyaiservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=trustyai.opendatahub.io.trustyai.opendatahub.io,resources=trustyaiservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=trustyai.opendatahub.io.trustyai.opendatahub.io,resources=trustyaiservices/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;watch;get;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=list;watch;create
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=list;get;watch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=list;get;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=list;watch;get;create;update;patch;delete

// getCommonLabels returns the service's common labels
func getCommonLabels(serviceName string) map[string]string {
	return map[string]string{
		"app":                        serviceName,
		"app.kubernetes.io/name":     serviceName,
		"app.kubernetes.io/instance": serviceName,
		"app.kubernetes.io/part-of":  serviceName,
		"app.kubernetes.io/version":  "0.1.0",
	}
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *TrustyAIServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	// Fetch the AppService instance
	instance := &trustyaiopendatahubiov1alpha1.TrustyAIService{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		// Handle error
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Check if the CR is being deleted
	if instance.DeletionTimestamp != nil {
		// CR is being deleted
		if containsString(instance.Finalizers, finalizerName) {
			// The finalizer is present, so we handle external dependency deletion
			if err := r.deleteExternalDependency(req.Name, req.Namespace, ctx); err != nil {
				// If fail to delete the external dependency here, return with error
				// so that it can be retried
				log.FromContext(ctx).Error(err, "Failed to delete external dependencies.")
				return ctrl.Result{}, err
			}

			// Remove the finalizer from the list and update it.
			instance.Finalizers = removeString(instance.Finalizers, finalizerName)
			if err := r.Update(ctx, instance); err != nil {
				log.FromContext(ctx).Error(err, "Failed to remove the finalizer.")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add the finalizer if it does not exist
	if !containsString(instance.Finalizers, finalizerName) {
		instance.Finalizers = append(instance.Finalizers, finalizerName)
		if err := r.Update(context.Background(), instance); err != nil {
			log.FromContext(ctx).Error(err, "Failed to add the finalizer.")
			return ctrl.Result{}, err
		}
	}

	// Update the instance status to Not Ready
	instance.Status.Phase = "Not Ready"
	instance.Status.Ready = corev1.ConditionFalse

	if err = r.updateCondition(ctx,
		instance, trustyAIAvailableConditionType, corev1.ConditionFalse,
		"TrustyAINotReady", "TrustyAI resources not ready"); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure PVC
	err = r.ensurePVC(ctx, instance)
	if err != nil {
		// PVC not found condition
		if err = r.updateCondition(ctx,
			instance, pvcAvailableConditionType, corev1.ConditionFalse,
			"PVCNotFound", "PersistentVolumeClaim not found"); err != nil {
			return ctrl.Result{}, err
		}
		log.FromContext(ctx).Error(err, "Error creating PVC storage.")

		// Update the instance status to Not Ready
		instance.Status.Phase = "Not Ready"
		instance.Status.Ready = corev1.ConditionFalse

		// If there was an error finding the PV, requeue the request
		log.FromContext(ctx).Error(err, "Could not find requested PersistentVolumeClaim.")
		return ctrl.Result{}, err

	} else {
		// Set the conditions appropriately
		if err = r.updateCondition(ctx,
			instance, pvcAvailableConditionType, corev1.ConditionTrue,
			"PVCFound", "PersistentVolumeClaim found"); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Ensure Deployment object
	err = r.ensureDeployment(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	allPodsRunning, err := r.allPodsRunning(ctx, req, instance)
	if err != nil {
		log.FromContext(ctx).Error(err, "Could not check if all pods are running.")
		return ctrl.Result{}, err
	}

	if !allPodsRunning {
		if err = r.updateCondition(ctx,
			instance, trustyAIAvailableConditionType, corev1.ConditionFalse,
			"PodsNotReady", "Not all pods are running"); err != nil {
			return ctrl.Result{}, err
		}
		log.FromContext(ctx).Info("Not all pods are running, requeue the reconcile request")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Fetch the TrustyAIService instance
	trustyAIServiceService := &trustyaiopendatahubiov1alpha1.TrustyAIService{}
	err = r.Get(ctx, req.NamespacedName, trustyAIServiceService)
	if err != nil {
		log.FromContext(ctx).Error(err, "Could not fetch service.")
		return ctrl.Result{}, err
	}

	// Create service
	service, err := r.reconcileService(trustyAIServiceService)
	if err != nil {
		// handle error
		return ctrl.Result{}, err
	}
	if err := r.Create(ctx, service); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Service already exists, no problem
			return ctrl.Result{}, nil
		}
		// handle error
		return ctrl.Result{}, err
	}

	// Service Monitor
	err = r.reconcileServiceMonitor(instance, ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create route
	err = r.reconcileRoute(instance, ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	// CR found, add or update the URL
	// Call the function to patch environment variables for Deployments that match the label
	_, err = r.patchEnvVarsByLabelForDeployments(ctx, req.Namespace, modelMeshLabelKey, modelMeshLabelValue, payloadProcessorName, req.Name, false)
	if err != nil {
		log.FromContext(ctx).Error(err, "Could not patch environment variables for Deployments.")
		// ModelMesh not configured condition
		if err = r.updateCondition(ctx, instance,
			modelMeshConfiguredConditionType, corev1.ConditionFalse,
			"ModelMeshNotConfigured", "Could not configure ModelMesh"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	} else {
		// ModelMesh configured condition
		if err = r.updateCondition(ctx, instance,
			modelMeshConfiguredConditionType, corev1.ConditionTrue,
			"ModelMeshConfigured", "ModelMesh configured"); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err = r.updateCondition(ctx, instance,
		trustyAIAvailableConditionType, corev1.ConditionTrue,
		"TrustyAIServiceReady", "TrustyAI service ready"); err != nil {
		return ctrl.Result{}, err
	}

	err = r.reconcileStatuses(instance, ctx)
	if err != nil {
		log.FromContext(ctx).Error(err, "Could not reconcile statuses.")
		return ctrl.Result{}, err
	}

	// Deployment already exists - don't requeue
	return ctrl.Result{}, nil
}

func (r *TrustyAIServiceReconciler) reconcileService(cr *trustyaiopendatahubiov1alpha1.TrustyAIService) (*corev1.Service, error) {
	annotations := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/path":   "/q/metrics",
		"prometheus.io/scheme": "http",
	}
	labels := getCommonLabels(cr.Name)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.Name,
			Namespace:   cr.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
	if err := ctrl.SetControllerReference(cr, service, r.Scheme); err != nil {
		return nil, err
	}
	return service, nil
}

func (r *TrustyAIServiceReconciler) reconcileServiceMonitor(cr *trustyaiopendatahubiov1alpha1.TrustyAIService, ctx context.Context) error {

	serviceMonitor := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceMonitorName,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"modelmesh-service": "modelmesh-serving",
			},
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{cr.Namespace},
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Interval:    "4s",
					Path:        "/q/metrics",
					HonorLabels: true,
					Scheme:      "http",
					Params: map[string][]string{
						"match[]": {
							`{__name__= "trustyai_spd"}`,
							`{__name__= "trustyai_dir"}`,
						},
					},
					MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
						{
							Action:       "keep",
							Regex:        "trustyai_.*",
							SourceLabels: []monitoringv1.LabelName{"__name__"},
						},
					},
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": cr.Name,
				},
			},
		},
	}

	// Set TrustyAIService instance as the owner and controller
	err := ctrl.SetControllerReference(cr, serviceMonitor, r.Scheme)
	if err != nil {
		return err
	}

	// Check if this ServiceMonitor already exists
	found := &monitoringv1.ServiceMonitor{}
	err = r.Get(ctx, types.NamespacedName{Name: serviceMonitor.Name, Namespace: serviceMonitor.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.FromContext(ctx).Info("Creating a new ServiceMonitor", "ServiceMonitor.Namespace", serviceMonitor.Namespace, "ServiceMonitor.Name", serviceMonitor.Name)
			err = r.Create(ctx, serviceMonitor)
			if err != nil {
				log.FromContext(ctx).Error(err, "Not found ServiceMonitor", "ServiceMonitor.Namespace", serviceMonitor.Namespace, "ServiceMonitor.Name", serviceMonitor.Name)
				return err
			}
		} else {
			log.FromContext(ctx).Error(err, "Couldn't create new ServiceMonitor", "ServiceMonitor.Namespace", serviceMonitor.Namespace, "ServiceMonitor.Name", serviceMonitor.Name)
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TrustyAIServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch ServingRuntime objects (not managed by this controller)
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &appsv1.Deployment{}, ".metadata.controller", func(rawObj client.Object) []string {
		// Grab the deployment object and extract the owner
		deployment := rawObj.(*appsv1.Deployment)
		owner := metav1.GetControllerOf(deployment)
		if owner == nil {
			return nil
		}
		// Retain ServingRuntimes only
		if owner.APIVersion != kserveapi.APIVersion || owner.Kind != "ServingRuntime" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&trustyaiopendatahubiov1alpha1.TrustyAIService{}).
		Complete(r)
}

// getTrustyAIImageAndTagFromConfigMap gets a custom TrustyAI image and tag from a ConfigMap in the operator's namespace
func (r *TrustyAIServiceReconciler) getImageAndTagFromConfigMap(ctx context.Context) (string, string, error) {
	if r.Namespace != "" {
		// Define the key for the ConfigMap
		configMapKey := types.NamespacedName{
			Namespace: r.Namespace,
			Name:      "trustyai-service-operator-config",
		}

		// Create an empty ConfigMap object
		var cm corev1.ConfigMap

		// Try to get the ConfigMap
		if err := r.Get(ctx, configMapKey, &cm); err != nil {
			if errors.IsNotFound(err) {
				// ConfigMap not found, fallback to default values
				return defaultImage, defaultTag, nil
			}
			// Other error occurred when trying to fetch the ConfigMap
			return defaultImage, defaultTag, fmt.Errorf("Error reading configmap %s", configMapKey)
		}

		// ConfigMap is found, extract the image and tag
		imageName, ok1 := cm.Data["trustyaiServiceImageName"]
		imageTag, ok2 := cm.Data["trustyaiServiceImageTag"]

		if !ok1 || !ok2 {
			// One or both of the keys are not present in the ConfigMap, return error
			return defaultImage, defaultTag, fmt.Errorf("configmap %s does not contain necessary keys", configMapKey)
		}

		// Return the image and tag
		return imageName, imageTag, nil
	} else {
		return defaultImage, defaultTag, nil
	}
}
