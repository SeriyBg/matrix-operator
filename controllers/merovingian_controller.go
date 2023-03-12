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

	matrixv1alpha1 "github.com/SeriyBg/matrix-operator/api/v1alpha1"
)

// MerovingianReconciler reconciles a Merovingian object
type MerovingianReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=matrix.operator.com,resources=merovingians,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=matrix.operator.com,resources=merovingians/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=matrix.operator.com,resources=merovingians/finalizers,verbs=update
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
func (r *MerovingianReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	m := &matrixv1alpha1.Merovingian{}
	err := r.Client.Get(ctx, req.NamespacedName, m)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "No Agent found!")
	}

	err = r.reconcilePermissions(ctx, m, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	ls := labels(m)

	dep := &v1.Deployment{
		ObjectMeta: metadata(m),
		Spec: v1.DeploymentSpec{
			Replicas: &[]int32{1}[0],
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metadata(m),
				Spec: corev1.PodSpec{
					ServiceAccountName: m.Name,
					Containers: []corev1.Container{{
						Name:  m.Name,
						Image: "sbishyr/matrix-merovingian:0.1",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8080,
							Name:          "http",
							Protocol:      corev1.ProtocolTCP,
						}},
					}},
				},
			},
		},
	}

	if err = createService(ctx, r.Client, m, logger, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	err = controllerutil.SetControllerReference(m, dep, r.Scheme)
	if err != nil {
		logger.Error(err, "Failed to set owner reference on ConfigMap")
		return ctrl.Result{}, err
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		return nil
	})
	if err != nil && errors.IsConflict(err) {
		return ctrl.Result{}, nil
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "Deployment", m.Name, "result", opResult)
	}
	return ctrl.Result{}, err
}

func (r *MerovingianReconciler) reconcilePermissions(ctx context.Context, m *matrixv1alpha1.Merovingian, logger logr.Logger) error {
	// Create ServiceAccount
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metadata(m),
	}
	err := controllerutil.SetControllerReference(m, serviceAccount, r.Scheme)
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
		logger.Info("Operation result", "ServiceAccount", m.Name, "result", opResult)
	}

	// Create ClusterRole
	roleName := fmt.Sprintf("%s-%s", m.Name, m.Namespace)
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleName,
			Labels: labels(m),
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
		logger.Info("Operation result", "ServiceAccount", m.Name, "result", opResult)
	}

	// Create ClusterRoleBinding
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleName,
			Labels: labels(m),
		},
	}
	opResult, err = controllerutil.CreateOrUpdate(ctx, r.Client, crb, func() error {
		crb.Subjects = []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      m.Name,
				Namespace: m.Namespace,
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
func (r *MerovingianReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&matrixv1alpha1.Merovingian{}).
		Owns(&v1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
