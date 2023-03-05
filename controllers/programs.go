package controllers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type MatrixProgram interface {
	metav1.Object
	ProgramName() string
}

func metadata(cr MatrixProgram) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      cr.GetName(),
		Namespace: cr.GetNamespace(),
		Labels:    labels(cr),
	}
}

func labels(cr MatrixProgram) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       cr.ProgramName(),
		"app.kubernetes.io/instance":   cr.GetName(),
		"app.kubernetes.io/managed-by": "matrix",
		"application":                  "matrix",
		"program":                      cr.GetName(),
	}
}

func createOrUpdate(ctx context.Context, c client.Client, obj client.Object, f controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	opResult, err := controllerutil.CreateOrUpdate(ctx, c, obj, f)
	if errors.IsAlreadyExists(err) {
		// Ignore "already exists" error.
		// Inside createOrUpdate() there's is a race condition between Get() and Create(), so this error is expected from time to time.
		return opResult, nil
	}
	return opResult, err
}

func createService(ctx context.Context, c client.Client, cr MatrixProgram, logger logr.Logger, s *runtime.Scheme) error {
	svc := &corev1.Service{
		ObjectMeta: metadata(cr),
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       8080,
				TargetPort: intstr.FromString("http"),
				Protocol:   corev1.ProtocolTCP,
			}},
			Selector: labels(cr),
		},
	}

	err := controllerutil.SetControllerReference(cr, svc, s)
	if err != nil {
		logger.Error(err, "Failed to set owner reference on ConfigMap")
		return err
	}
	opResult, err := controllerutil.CreateOrUpdate(ctx, c, svc, func() error {
		return nil
	})
	if err != nil && errors.IsConflict(err) {
		return nil
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "Service", cr.GetName(), "result", opResult)
	}
	return err
}
