package tas

import (
	"context"

	routev1 "github.com/openshift/api/route/v1"
	trustyaiopendatahubiov1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/tas/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	routeTemplatePath = "service/route.tmpl.yaml"
)

func (r *TrustyAIServiceReconciler) createRouteObject(ctx context.Context, instance *trustyaiopendatahubiov1alpha1.TrustyAIService) (*routev1.Route, error) {

	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				"trustyai-service-name": instance.Name,
			},
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind:   "Service",
				Name:   instance.Name + "-tls",
				Weight: new(int32),
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString(OAuthServicePortName),
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationEdge,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			},
		},
	}

	*route.Spec.To.Weight = 100

	if err := ctrl.SetControllerReference(instance, route, r.Scheme); err != nil {
		return nil, err
	}

	return route, nil
}

// Reconcile will manage the creation, update and deletion of the route returned
// by the newRoute function
func (r *TrustyAIServiceReconciler) reconcileRouteAuth(instance *trustyaiopendatahubiov1alpha1.TrustyAIService,
	ctx context.Context, newRoute func(context.Context, *trustyaiopendatahubiov1alpha1.TrustyAIService) (*routev1.Route, error)) error {

	// Generate the desired route
	desiredRoute, err := newRoute(ctx, instance)
	if err != nil {
		return err
	}

	// Create the route if it does not already exist
	foundRoute := &routev1.Route{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      desiredRoute.Name,
		Namespace: instance.Namespace,
	}, foundRoute)
	if err != nil {
		if errors.IsNotFound(err) {
			log.FromContext(ctx).Info("Creating Route")
			err = ctrl.SetControllerReference(instance, desiredRoute, r.Scheme)
			if err != nil {
				log.FromContext(ctx).Error(err, "Unable to add OwnerReference to the Route")
				return err
			}
			err = r.Create(ctx, desiredRoute)
			if err != nil && !errors.IsAlreadyExists(err) {
				log.FromContext(ctx).Error(err, "Unable to create the Route")
				return err
			}
		} else {
			log.FromContext(ctx).Error(err, "Unable to fetch the Route")
			return err
		}
	} else {
		if updateRoute(foundRoute, desiredRoute) {
			foundRoute.Spec = desiredRoute.Spec
			err = r.Update(ctx, foundRoute)
			if err != nil {
				log.FromContext(ctx).Error(err, "Unable to update the Route")
				return err
			}
			log.FromContext(ctx).Info("Route updated")
		}
	}

	return nil
}

// updateRoute reconciles route
func updateRoute(foundRoute, desiredRoute *routev1.Route) bool {
	if foundRoute.Spec.TLS == nil && desiredRoute.Spec.TLS != nil {
		return true
	}
	if foundRoute.Spec.TLS != nil && desiredRoute.Spec.TLS != nil {
		if foundRoute.Spec.TLS.DestinationCACertificate != desiredRoute.Spec.TLS.DestinationCACertificate {
			return true
		}
		if foundRoute.Spec.TLS.Termination != desiredRoute.Spec.TLS.Termination {
			return true
		}
		if foundRoute.Spec.TLS.InsecureEdgeTerminationPolicy != desiredRoute.Spec.TLS.InsecureEdgeTerminationPolicy {
			return true
		}
	}

	if foundRoute.Spec.To.Name != desiredRoute.Spec.To.Name {
		return true
	}

	if foundRoute.Spec.Port != nil && desiredRoute.Spec.Port != nil {
		if foundRoute.Spec.Port.TargetPort != desiredRoute.Spec.Port.TargetPort {
			return true
		}
	} else if foundRoute.Spec.Port != nil || desiredRoute.Spec.Port != nil {
		return true
	}

	return false
}

// ReconcileRoute will manage the creation, update and deletion of the
// TLS route when the service is reconciled
func (r *TrustyAIServiceReconciler) ReconcileRoute(
	instance *trustyaiopendatahubiov1alpha1.TrustyAIService, ctx context.Context) error {
	return r.reconcileRouteAuth(instance, ctx, r.createRouteObject)
}

func (r *TrustyAIServiceReconciler) checkRouteReady(ctx context.Context, cr *trustyaiopendatahubiov1alpha1.TrustyAIService) (bool, error) {
	existingRoute := &routev1.Route{}

	err := r.Client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, existingRoute)
	if err != nil {
		log.FromContext(ctx).Info("Unable to find the Route")
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	for _, ingress := range existingRoute.Status.Ingress {
		for _, condition := range ingress.Conditions {
			if condition.Type == routev1.RouteAdmitted && condition.Status == corev1.ConditionTrue {
				// The Route is admitted, so it's ready
				return true, nil
			}
		}
	}

	return false, nil
}
