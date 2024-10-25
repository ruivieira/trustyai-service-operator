package lmes

import (
	"context"

	"github.com/go-logr/logr"
	lmesv1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/lmes/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *LMEvalJobReconciler) handleManagedPVC(ctx context.Context, log logr.Logger, job *lmesv1alpha1.LMEvalJob) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *LMEvalJobReconciler) handleCustomPVC(ctx context.Context, log logr.Logger, job *lmesv1alpha1.LMEvalJob) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *LMEvalJobReconciler) handleExistingPVC(ctx context.Context, log logr.Logger, job *lmesv1alpha1.LMEvalJob) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
