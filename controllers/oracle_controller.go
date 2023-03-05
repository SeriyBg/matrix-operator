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

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	matrixv1alpha1 "github.com/SeriyBg/matrix-operator/api/v1alpha1"
)

// OracleReconciler reconciles a Oracle object
type OracleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=matrix.operator.com,resources=oracles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=matrix.operator.com,resources=oracles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=matrix.operator.com,resources=oracles/finalizers,verbs=update
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
func (r *OracleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	o := &matrixv1alpha1.Oracle{}
	err := r.Client.Get(ctx, req.NamespacedName, o)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "No Agent found!")
	}

	ls := labels(o)

	dep := &v1.Deployment{
		ObjectMeta: metadata(o),
		Spec: v1.DeploymentSpec{
			Replicas: &[]int32{1}[0],
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metadata(o),
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  o.Name,
						Image: "sbishyr/matrix-oracle:0.1",
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
	err = controllerutil.SetControllerReference(o, dep, r.Scheme)
	if err != nil {
		logger.Error(err, "Failed to set owner reference on ConfigMap")
		return ctrl.Result{}, err
	}

	if o.Spec.Config != nil {
		err := r.reconcileConfigMap(ctx, o, logger)
		if err != nil {
			return ctrl.Result{}, err
		}
		dep.Spec.Template.Spec.Volumes = []corev1.Volume{{
			Name: "oracle-config",
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: o.Name,
				},
			}},
		}}
		dep.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
			Name:      "oracle-config",
			MountPath: "data/config",
		}}
		dep.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{
			Name:  "EXTERNAL_CONFIGURATION",
			Value: "/data/config/oracle-config.yaml",
		}}
	}

	err = createService(ctx, r.Client, o, logger, r.Scheme)
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
		logger.Info("Operation result", "Deployment", o.Name, "result", opResult)
	}

	return ctrl.Result{}, err
}

func (r *OracleReconciler) reconcileConfigMap(ctx context.Context, o *matrixv1alpha1.Oracle, logger logr.Logger) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metadata(o),
	}
	err := controllerutil.SetControllerReference(o, cm, r.Scheme)
	if err != nil {
		logger.Error(err, "Failed to set owner reference on ConfigMap")
		return err
	}
	opResult, err := createOrUpdate(ctx, r.Client, cm, func() error {
		yml, err := yaml.Marshal(o.Spec.Config)
		if err != nil {
			return err
		}
		cm.Data = map[string]string{"oracle-config.yaml": string(yml)}
		return err
	})
	if err != nil {
		return err
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "ConfigMap", o.Name, "result", opResult)
	}
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *OracleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&matrixv1alpha1.Oracle{}).
		Owns(&v1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
