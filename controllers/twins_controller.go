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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	matrixv1alpha1 "github.com/SeriyBg/matrix-operator/api/v1alpha1"
)

// TwinsReconciler reconciles a Twins object
type TwinsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=matrix.operator.com.matrix.operator.com,resources=twins,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=matrix.operator.com.matrix.operator.com,resources=twins/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=matrix.operator.com.matrix.operator.com,resources=twins/finalizers,verbs=update
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
func (r *TwinsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	tw := &matrixv1alpha1.Twins{}
	err := r.Client.Get(ctx, req.NamespacedName, tw)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "No Agent found!")
	}

	err = r.addFinalizer(ctx, tw, logger)
	if err != nil {
		return ctrl.Result{}, err
	}
	ct := corev1.Container{
		Name:  tw.Name,
		Image: "sbishyr/matrix-merovingian:0.1",
		Ports: []corev1.ContainerPort{{
			ContainerPort: 8080,
			Name:          "http",
			Protocol:      corev1.ProtocolTCP,
		}},
	}
	if tw.GetDeletionTimestamp() != nil {
		// Execute finalizer's pre-delete function to cleanup ClusterRole
		err = r.executeFinalizer(ctx, tw, ct)
		if err != nil {
			logger.Error(err, "Finalizer execution failed")
			return ctrl.Result{}, err
		}
		logger.V(1).Info("Finalizer's pre-delete function executed successfully and the finalizer removed from custom resource")
		return ctrl.Result{}, nil
	}

	ls := labels(tw)

	dep := &v1.Deployment{
		ObjectMeta: metadata(tw),
		Spec: v1.DeploymentSpec{
			Replicas: &[]int32{1}[0],
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metadata(tw),
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{ct},
				},
			},
		},
	}
	err = controllerutil.SetControllerReference(tw, dep, r.Scheme)
	if err != nil {
		logger.Error(err, "Failed to set owner reference on ConfigMap")
		return ctrl.Result{}, err
	}

	if err = createService(ctx, r.Client, tw, logger, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		return nil
	})
	if err != nil && errors.IsConflict(err) {
		return ctrl.Result{}, nil
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "Deployment", tw.Name, "result", opResult)
	}
	return ctrl.Result{}, err
}

func (r *TwinsReconciler) addFinalizer(ctx context.Context, cr client.Object, logger logr.Logger) error {
	if !controllerutil.ContainsFinalizer(cr, "matrix.com/finalizer") {
		controllerutil.AddFinalizer(cr, "matrix.com/finalizer")
		err := r.Update(ctx, cr)
		if err != nil {
			logger.Error(err, "Failed to add finalizer into custom resource")
			return err
		}
		logger.V(1).Info("Finalizer added into custom resource successfully")
	}
	return nil
}

func (r *TwinsReconciler) executeFinalizer(ctx context.Context, cr client.Object, ct corev1.Container) error {
	m := &matrixv1alpha1.MerovingianList{}
	err := r.List(ctx, m, client.InNamespace(cr.GetNamespace()))
	if err != nil {
		return err
	}
	for _, item := range m.Items {
		err = r.exileTo(ctx, &item, ct)
		if err != nil {
			return err
		}
	}
	controllerutil.RemoveFinalizer(cr, "matrix.com/finalizer")
	err = r.Update(ctx, cr)
	if err != nil {
		return err
	}
	return nil
}

func (r *TwinsReconciler) exileTo(ctx context.Context, item *matrixv1alpha1.Merovingian, ct corev1.Container) error {
	md := &v1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: item.Name, Namespace: item.Namespace}, md)
	if err != nil {
		return err
	}
	ct.Env = []corev1.EnvVar{{
		Name:  "SERVER_PORT",
		Value: fmt.Sprintf("%d", nextPort(md.Spec.Template.Spec.Containers)),
	}}
	md.Spec.Template.Spec.Containers = append(md.Spec.Template.Spec.Containers, ct)
	err = r.Update(ctx, md)
	if err != nil {
		return err
	}

	item.Status.ExiledCount = int32(len(md.Spec.Template.Spec.Containers) - 1)
	exiled := make([]string, 0, item.Status.ExiledCount)
	for _, c := range md.Spec.Template.Spec.Containers {
		if c.Name == item.Name {
			continue
		}
		exiled = append(exiled, c.Name)
	}
	item.Status.Exiled = exiled
	err = r.Status().Update(ctx, item)
	return err
}

func nextPort(c []corev1.Container) int32 {
	lastC := c[len(c)-1]
	port := lastC.Ports[0]
	return port.ContainerPort + 1
}

// SetupWithManager sets up the controller with the Manager.
func (r *TwinsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&matrixv1alpha1.Twins{}).
		Owns(&v1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
