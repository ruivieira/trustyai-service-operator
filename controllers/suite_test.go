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
	routev1 "github.com/openshift/api/route/v1"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/require"
	trustyaiopendatahubiov1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func TestTrustyAIServiceReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()

	// Create a fake TrustyAIService instance
	instance := &trustyaiopendatahubiov1alpha1.TrustyAIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
		},
		Spec: trustyaiopendatahubiov1alpha1.TrustyAIServiceSpec{
			Storage: trustyaiopendatahubiov1alpha1.StorageSpec{
				Format: "PVC",
				Folder: "/data",
				Size:   "1Gi",
			},
			Data: trustyaiopendatahubiov1alpha1.DataSpec{
				Filename: "data.csv",
				Format:   "CSV",
			},
			Metrics: trustyaiopendatahubiov1alpha1.MetricsSpec{
				Schedule: "5s",
			},
		},
		Status: trustyaiopendatahubiov1alpha1.TrustyAIServiceStatus{
			Phase: "Not Ready",
			Ready: corev1.ConditionFalse,
		},
	}

	// Create a scheme for the fake client
	scheme := runtime.NewScheme()

	// Add core Kubernetes types to the scheme
	err := corev1.AddToScheme(scheme)
	require.NoError(t, err)

	// Add Deployment to the scheme
	err = appsv1.AddToScheme(scheme)
	require.NoError(t, err)

	// Add ServiceMonitor to the scheme
	err = monitorv1.AddToScheme(scheme)
	require.NoError(t, err)

	// Add Route to the scheme
	err = routev1.AddToScheme(scheme)
	require.NoError(t, err)

	// Make sure that the TrustyAIService type is added to the scheme
	err = trustyaiopendatahubiov1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	// Create a fake client to mock API calls, and use the scheme created above
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(instance).Build()

	// Create a ReconcileTrustyAIService object with the mock client
	r := &TrustyAIServiceReconciler{
		Client: cl,
		Scheme: scheme,
	}

	_, err = r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// Fetch the updated TrustyAIService instance
	updatedInstance := &trustyaiopendatahubiov1alpha1.TrustyAIService{}
	err = cl.Get(ctx, types.NamespacedName{Name: "test-instance", Namespace: "default"}, updatedInstance)
	require.NoError(t, err)

	// Check that the status has been updated to Ready
	require.Equal(t, "Ready", updatedInstance.Status.Phase)
	require.Equal(t, corev1.ConditionTrue, updatedInstance.Status.Ready)
}
