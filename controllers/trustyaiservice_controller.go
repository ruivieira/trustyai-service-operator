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
	kservev1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	trustyaiopendatahubiov1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var ErrPVCNotReady = goerrors.New("PVC is not ready")

// TrustyAIServiceReconciler reconciles a TrustyAIService object
type TrustyAIServiceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Namespace     string
	EventRecorder record.EventRecorder
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
//+kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=list;watch;get;update;patch
//+kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices/finalizers,verbs=list;watch;get;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update

// getCommonLabels returns the service's common labels
func getCommonLabels(serviceName string) map[string]string {
	return map[string]string{
		"app":                        serviceName,
		"app.kubernetes.io/name":     serviceName,
		"app.kubernetes.io/instance": serviceName,
		"app.kubernetes.io/part-of":  componentName,
		"app.kubernetes.io/version":  "0.1.0",
	}
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *TrustyAIServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	// Fetch the AppService instance
	instance := &trustyaiopendatahubiov1alpha1.TrustyAIService{}
	err := r.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		// Handle error
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			return DoNotRequeue()
		}
		// Error reading the object - requeue the request.
		return RequeueWithError(err)
	}

	// Check if the CR is being deleted
	if instance.DeletionTimestamp != nil {
		// CR is being deleted
		if containsString(instance.Finalizers, finalizerName) {
			// The finalizer is present, so we handle external dependency deletion
			if err := r.deleteExternalDependency(req.Name, instance, req.Namespace, ctx); err != nil {
				// If fail to delete the external dependency here, return with error
				// so that it can be retried
				return RequeueWithErrorMessage(ctx, err, "Failed to delete external dependencies.")
			}

			// Remove the finalizer from the list and update it.
			instance.Finalizers = removeString(instance.Finalizers, finalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return RequeueWithErrorMessage(ctx, err, "Failed to remove the finalizer.")
			}
		}
		return DoNotRequeue()
	}

	// Add the finalizer if it does not exist
	if !containsString(instance.Finalizers, finalizerName) {
		instance.Finalizers = append(instance.Finalizers, finalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return RequeueWithErrorMessage(ctx, err, "Failed to add the finalizer.")
		}
	}

	// CR found, add or update the URL
	// Call the function to patch environment variables for Deployments that match the label
	shouldContinue, err := r.handleInferenceServices(ctx, instance, req.Namespace, modelMeshLabelKey, modelMeshLabelValue, payloadProcessorName, req.Name, false)
	if err != nil {
		return RequeueWithErrorMessage(ctx, err, "Could not patch environment variables for Deployments.")
	}
	if !shouldContinue {
		return RequeueWithDelayMessage(ctx, time.Minute, "Not all replicas are ready, requeue the reconcile request")
	}

	// Ensure PVC
	err = r.ensurePVC(ctx, instance)
	if err != nil {
		// PVC not found condition
		log.FromContext(ctx).Error(err, "Error creating PVC storage.")
		_, updateErr := r.updateStatus(ctx, instance, UpdatePVCNotAvailable)
		if updateErr != nil {
			return RequeueWithErrorMessage(ctx, err, "Failed to update status")
		}

		// If there was an error finding the PV, requeue the request
		return RequeueWithErrorMessage(ctx, err, "Could not find requested PersistentVolumeClaim.")

	}

	// Ensure Deployment object
	err = r.ensureDeployment(ctx, instance)
	if err != nil {
		return RequeueWithError(err)
	}

	// Fetch the TrustyAIService instance
	trustyAIServiceService := &trustyaiopendatahubiov1alpha1.TrustyAIService{}
	err = r.Get(ctx, req.NamespacedName, trustyAIServiceService)
	if err != nil {
		return RequeueWithErrorMessage(ctx, err, "Could not fetch service.")
	}

	// Create service
	service, err := r.reconcileService(trustyAIServiceService)
	if err != nil {
		// handle error
		return RequeueWithError(err)
	}
	if err := r.Create(ctx, service); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Service already exists, no problem
		} else {
			// handle any other error
			return RequeueWithError(err)
		}
	}

	// Local Service Monitor
	err = r.ensureLocalServiceMonitor(instance, ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Central Service Monitor
	err = r.ensureCentralServiceMonitor(ctx)
	if err != nil {
		return RequeueWithError(err)
	}

	// Create route
	err = r.reconcileRoute(instance, ctx)
	if err != nil {
		// Could not create Route object, update status and return.
		_, updateErr := r.updateStatus(ctx, instance, UpdateRouteNotAvailable)
		if updateErr != nil {
			return RequeueWithErrorMessage(ctx, err, "Failed to update status")
		}
		return RequeueWithErrorMessage(ctx, err, "Failed to get or create Route")
	}

	// Ensure PVC
	pvcReady, err := r.checkPVCReady(ctx, instance)
	if err != nil || !pvcReady {
		// PVC not ready, requeue
		log.FromContext(ctx).Info("PVC not ready, requeuing")
		return Requeue()
	}

	// Ensure Deployment
	deploymentReady, err := r.checkDeploymentReady(ctx, instance)
	if err != nil || !deploymentReady {
		// Deployment not ready, requeue
		log.FromContext(ctx).Info("Deployment not ready, requeuing")
		return Requeue()
	}

	// Ensure Route
	routeReady, err := r.checkRouteReady(ctx, instance)
	if err != nil || !routeReady {
		// Route not ready, requeue
		log.FromContext(ctx).Info("Route not ready, requeuing")
		return Requeue()
	}

	inferenceServiceReady, err := r.checkAllInferenceServicesReady(ctx, instance.Namespace)

	// All checks passed, resources are ready
	if pvcReady && deploymentReady && routeReady {
		log.FromContext(ctx).Info("All resources ready")
		_, updateErr := r.updateStatus(ctx, instance, func(saved *trustyaiopendatahubiov1alpha1.TrustyAIService) {

			if inferenceServiceReady {
				log.FromContext(ctx).Info("Inference service ready")
				UpdateInferenceServicePresent(saved)
			} else {
				log.FromContext(ctx).Info("Inference service not ready")
				UpdateInferenceServiceNotPresent(saved)
			}

			UpdatePVCAvailable(saved)
			UpdateRouteAvailable(saved)
			UpdateTrustyAIServiceAvailable(saved)
			log.FromContext(ctx).Info("TrustyAI service ready")
			saved.Status.Phase = "Ready"
			saved.Status.Ready = corev1.ConditionTrue
		})
		if updateErr != nil {
			return RequeueWithErrorMessage(ctx, err, "Failed to update status")
		}
	} else {
		_, updateErr := r.updateStatus(ctx, instance, func(saved *trustyaiopendatahubiov1alpha1.TrustyAIService) {

			if inferenceServiceReady {
				log.FromContext(ctx).Info("Inference service ready")
				UpdateInferenceServicePresent(saved)
			} else {
				log.FromContext(ctx).Info("Inference service not ready")
				UpdateInferenceServiceNotPresent(saved)
			}

			if pvcReady {
				log.FromContext(ctx).Info("PVC ready")
				UpdatePVCAvailable(saved)
			} else {
				log.FromContext(ctx).Info("PVC not ready")
				UpdatePVCNotAvailable(saved)
			}
			if routeReady {
				log.FromContext(ctx).Info("Route ready")
				UpdateRouteAvailable(saved)
			} else {
				log.FromContext(ctx).Info("Route not ready")
				UpdateRouteNotAvailable(saved)
			}
			log.FromContext(ctx).Info("TrustyAI service not ready")
			UpdateTrustyAIServiceNotAvailable(saved)
			saved.Status.Phase = "Ready"
			saved.Status.Ready = corev1.ConditionFalse
		})
		if updateErr != nil {
			return RequeueWithErrorMessage(ctx, err, "Failed to update status")
		}
	}
	// Deployment already exists - requeue the request with a delay
	log.FromContext(ctx).Info("requeuing")
	return RequeueWithDelay(defaultRequeueDelay)
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
		if owner.APIVersion != kservev1beta1.APIVersion || owner.Kind != "ServingRuntime" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&trustyaiopendatahubiov1alpha1.TrustyAIService{}).
		Watches(&source.Kind{Type: &kservev1beta1.InferenceService{}}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &kservev1alpha1.ServingRuntime{}}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

// getTrustyAIImageAndTagFromConfigMap gets a custom TrustyAI image and tag from a ConfigMap in the operator's namespace
func (r *TrustyAIServiceReconciler) getImageFromConfigMap(ctx context.Context) (string, error) {
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
				return defaultImage, nil
			}
			// Other error occurred when trying to fetch the ConfigMap
			return defaultImage, fmt.Errorf("error reading configmap %s", configMapKey)
		}

		// ConfigMap is found, extract the image and tag
		image, ok := cm.Data["trustyaiServiceImage"]

		if !ok {
			// One or both of the keys are not present in the ConfigMap, return error
			return defaultImage, fmt.Errorf("configmap %s does not contain necessary keys", configMapKey)
		}

		// Return the image and tag
		return image, nil
	} else {
		return defaultImage, nil
	}
}
