/*
Copyright 2022.

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
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	matrixcomv1alpha1 "github.com/SeriyBg/matrix-operator/api/v1alpha1"
)

// ArchitectReconciler reconciles a Architect object
type ArchitectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=matrix.operator.com.matrix.operator.com,resources=architects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=matrix.operator.com.matrix.operator.com,resources=architects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=matrix.operator.com.matrix.operator.com,resources=architects/finalizers,verbs=update
// ClusterRole inherited from Hazelcast ClusterRole
//+kubebuilder:rbac:groups="",resources=endpoints;pods;nodes;services,verbs=get;list
// Role related to Reconcile()
//+kubebuilder:rbac:groups="",resources=events;services;serviceaccounts;configmaps;pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets;deployments,verbs=get;list;watch;create;update;patch;delete
// ClusterRole related to Reconcile()
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ArchitectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	a := &matrixcomv1alpha1.Architect{}
	err := r.Client.Get(ctx, req.NamespacedName, a)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "No Agent found!")
	}

	err = r.reconcilePermissions(ctx, a, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	ls := labels(a)
	dep := &v1.Deployment{
		ObjectMeta: metadata(a),
		Spec: v1.DeploymentSpec{
			Replicas: &[]int32{1}[0],
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metadata(a),
				Spec: corev1.PodSpec{
					ServiceAccountName: a.Name,
					Containers: []corev1.Container{{
						Name:  a.Name,
						Image: "sbishyr/matrix-architect:0.1",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8080,
							Name:          "http",
							Protocol:      corev1.ProtocolTCP,
						}},
						Env: []corev1.EnvVar{{
							Name:  "MATRIX_NAMESPACE",
							Value: matrixNamespace(a),
						}},
					}},
				},
			},
		},
	}
	err = controllerutil.SetControllerReference(a, dep, r.Scheme)
	if err != nil {
		logger.Error(err, "Failed to set owner reference on ConfigMap")
		return ctrl.Result{}, err
	}

	err = createService(ctx, r.Client, a, logger, r.Scheme)
	if err != nil {
		return ctrl.Result{}, err
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		return nil
	})
	if err != nil && errors.IsConflict(err) {
		return ctrl.Result{}, nil
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "Deployment", a.Name, "result", opResult)
	}
	return ctrl.Result{}, err
}

func matrixNamespace(a *matrixcomv1alpha1.Architect) string {
	if a.Spec.MatrixNamespace != "" {
		return a.Spec.MatrixNamespace
	}
	return a.Namespace
}

func (r *ArchitectReconciler) reconcilePermissions(ctx context.Context, a *matrixcomv1alpha1.Architect, logger logr.Logger) error {
	// Create ServiceAccount
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metadata(a),
	}
	err := controllerutil.SetControllerReference(a, serviceAccount, r.Scheme)
	if err != nil {
		logger.Error(err, "Failed to set owner reference on ServiceAccount")
		return err
	}
	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, serviceAccount, func() error {
		return nil
	})
	if err != nil && errors.IsConflict(err) {
		return nil
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "ServiceAccount", a.Name, "result", opResult)
	}

	// Create ClusterRole
	roleName := fmt.Sprintf("%s-%s", a.Name, a.Namespace)
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleName,
			Labels: labels(a),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "pods", "nodes", "services"},
				Verbs:     []string{"get", "list"},
			},
		},
	}
	opResult, err = controllerutil.CreateOrUpdate(ctx, r.Client, clusterRole, func() error {
		return nil
	})
	if err != nil && errors.IsConflict(err) {
		return nil
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "ServiceAccount", a.Name, "result", opResult)
	}

	// Create ClusterRoleBinding
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleName,
			Labels: labels(a),
		},
	}
	opResult, err = controllerutil.CreateOrUpdate(ctx, r.Client, crb, func() error {
		crb.Subjects = []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      a.Name,
				Namespace: a.Namespace,
			},
		}
		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		}

		return nil
	})
	if err != nil && errors.IsConflict(err) {
		return nil
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "ClusterRoleBinding", roleName, "result", opResult)
	}
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ArchitectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&matrixcomv1alpha1.Architect{}).
		Owns(&v1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
