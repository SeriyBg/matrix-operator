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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	matrixv1beta1 "github.com/SeriyBg/matrix-operator/api/v1beta1"
)

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	agentsWatchers map[types.NamespacedName]*agentWatcher
	httpClient     *http.Client
}

func NewAgentReconciler(client client.Client, scheme *runtime.Scheme) *AgentReconciler {
	return &AgentReconciler{
		Client:         client,
		Scheme:         scheme,
		agentsWatchers: make(map[types.NamespacedName]*agentWatcher),
		httpClient: &http.Client{
			Timeout: time.Minute / 5,
		},
	}
}

type agentWatcher struct {
	done chan bool
	t    *time.Ticker
}

func (a *agentWatcher) Stop() {
	a.t.Stop()
	a.done <- true
}

type AgentHealth struct {
	Name   matrixv1beta1.AgentName `json:"name"`
	Health int                     `json:"health"`
}

//+kubebuilder:rbac:groups=matrix.operator.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=matrix.operator.com,resources=agents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=matrix.operator.com,resources=agents/finalizers,verbs=update
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
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	a := &matrixv1beta1.Agent{}
	err := r.Client.Get(ctx, req.NamespacedName, a)
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.tryInitAgent(ctx, req, logger)
			return ctrl.Result{}, err
		}
		logger.Error(err, "No Agent found!")
	}

	ls := labels(a)
	replicas := int32(len(a.Spec.Agents))
	sts := &appv1.StatefulSet{
		ObjectMeta: metadata(a),
		Spec: appv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Replicas:    &replicas,
			ServiceName: a.Name,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  a.Name,
						Image: "sbishyr/matrix-agent:0.3",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8080,
							Name:          "http",
							Protocol:      corev1.ProtocolTCP,
						}},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/health",
									Port:   intstr.FromInt(8080),
									Scheme: corev1.URISchemeHTTP,
								},
							},
							InitialDelaySeconds: 0,
							TimeoutSeconds:      10,
							PeriodSeconds:       10,
							SuccessThreshold:    1,
							FailureThreshold:    10,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/health",
									Port:   intstr.FromInt(8080),
									Scheme: corev1.URISchemeHTTP,
								},
							},
							InitialDelaySeconds: 0,
							TimeoutSeconds:      10,
							PeriodSeconds:       10,
							SuccessThreshold:    1,
							FailureThreshold:    10,
						},
					}},
				},
			},
		},
	}
	err = createServicePerPod(ctx, r.Client, a, logger, r.Scheme)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = controllerutil.SetControllerReference(a, sts, r.Scheme)
	if err != nil {
		logger.Error(err, "Failed to set owner reference on ConfigMap")
		return ctrl.Result{}, err
	}
	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		return nil
	})
	if err != nil && errors.IsConflict(err) {
		return ctrl.Result{}, nil
	}
	if opResult != controllerutil.OperationResultNone {
		logger.Info("Operation result", "Statefulset", a.Name, "result", opResult)
	}
	return ctrl.Result{}, err
}

func servicePerPodSelector(i int, agent *matrixv1beta1.Agent) map[string]string {
	ls := labels(agent)
	ls["statefulset.kubernetes.io/pod-name"] = agent.Name + fmt.Sprintf("-%d", i)
	return ls
}

func createServicePerPod(ctx context.Context, c client.Client, agent *matrixv1beta1.Agent, logger logr.Logger, s *runtime.Scheme) error {
	for i, ag := range agent.Spec.Agents {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      agent.Name + "-" + strings.ToLower(string(ag.Name)),
				Namespace: agent.GetNamespace(),
				Labels:    labels(agent),
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				}},
				Selector: servicePerPodSelector(i, agent),
			},
		}
		err := controllerutil.SetControllerReference(agent, svc, s)
		if err != nil {
			logger.Error(err, "Failed to set owner reference on ConfigMap")
			return err
		}
		opResult, err := controllerutil.CreateOrUpdate(ctx, c, svc, func() error {
			return nil
		})
		if err != nil {
			if errors.IsConflict(err) {
				return nil
			}
			return err
		}
		if opResult != controllerutil.OperationResultNone {
			logger.Info("Operation result", "Service", agent.Name, "result", opResult)
		}
	}
	return nil
}

func (r *AgentReconciler) podUpdates(pod client.Object) []reconcile.Request {
	p, ok := pod.(*corev1.Pod)
	if !ok {
		return []reconcile.Request{}
	}

	if p.Status.Phase != corev1.PodRunning {
		return []reconcile.Request{}
	}

	if !p.Status.ContainerStatuses[0].Ready {
		time.Sleep(5 * time.Second)
	}

	if p.Labels["app.kubernetes.io/managed-by"] != "matrix" || p.Labels["app.kubernetes.io/name"] != "agent" {
		return []reconcile.Request{}
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      p.GetName(),
				Namespace: p.GetNamespace(),
			},
		},
	}
}

func (r *AgentReconciler) tryInitAgent(ctx context.Context, req ctrl.Request, logger logr.Logger) error {
	pod := &corev1.Pod{}
	err := r.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	a := &matrixv1beta1.Agent{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      pod.Labels["app.kubernetes.io/instance"],
		Namespace: req.Namespace,
	}, a)
	if err != nil {
		if errors.IsNotFound(err) {
			if w, ok := r.agentsWatchers[req.NamespacedName]; ok {
				w.Stop()
				delete(r.agentsWatchers, req.NamespacedName)
			}
			return nil
		}
		return err
	}

	url := pod.Status.PodIP + ":8080"
	aName := agentName(a.Spec.Agents, pod.Name)

	err = r.initAgentName(url, aName, logger)
	if err != nil {
		return err
	}

	err = r.initAgentHealth(agentHealth(a, aName), aName, url, logger)
	if err != nil {
		return err
	}

	if t, ok := r.agentsWatchers[req.NamespacedName]; ok {
		t.Stop()
		delete(r.agentsWatchers, req.NamespacedName)
	}
	aw := &agentWatcher{
		t:    time.NewTicker(5 * time.Second),
		done: make(chan bool),
	}
	go func() {
		for {
			select {
			case <-aw.done:
				return
			case _ = <-aw.t.C:
				err = r.Get(ctx, types.NamespacedName{
					Name:      pod.Labels["app.kubernetes.io/instance"],
					Namespace: req.Namespace,
				}, a)
				logger.Info("Executing Agent status check", "Pod", pod.Name)
				b, err := http.Get("http://" + url + "/health")
				if err != nil || b.StatusCode != 200 {
					logger.Error(err, "Unable to get agent health")
					continue
				}
				h, err := io.ReadAll(b.Body)
				if err != nil {
					logger.Error(err, "Unable to read health response data")
					continue
				}
				health := &AgentHealth{}
				err = json.Unmarshal(h, health)
				logger.Info("Found agent status", "health", health)
				if err != nil {
					logger.Error(err, "Unable to unmarshal health response data")
					continue
				}
				foundAgent := false
				agentsAlive := 0

				agentsStatuses := make([]matrixv1beta1.SingeAgentStatus, 0, 0)
				for _, agent := range a.Status.Agents {
					if agent.Name == health.Name {
						agent.Health = int32(health.Health)
						foundAgent = true
					}
					agentsStatuses = append(agentsStatuses, agent)
					if health.Health > 0 {
						agentsAlive += 1
					}
				}
				if !foundAgent {
					agentsStatuses = append(agentsStatuses, matrixv1beta1.SingeAgentStatus{
						Name:   health.Name,
						Health: int32(health.Health),
					})
					if health.Health > 0 {
						agentsAlive += 1
					}
				}
				a.Status.Agents = agentsStatuses
				logger.Info("Agents statuses", "agents", a.Status.Agents)
				a.Status.AgentsAlive = fmt.Sprintf("%d/%d", agentsAlive, len(a.Spec.Agents))
				err = r.Status().Update(ctx, a)
				if err != nil {
					logger.Error(err, "Error while updating agent status")
				}
			}
		}
	}()
	r.agentsWatchers[req.NamespacedName] = aw

	return nil
}

func (r *AgentReconciler) initAgentHealth(health int32, aName string, url string, logger logr.Logger) error {
	post, err := http.NewRequest("POST", "http://"+url+"/health", strings.NewReader(fmt.Sprintf("%d", health)))
	if err != nil {
		return err
	}
	post.Header = http.Header{"Content-Type": []string{"text/plain"}}
	res, err := r.httpClient.Do(post)
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(res.Body)
	body := buf.String()
	logger.Info("Response after init agent health",
		"StatusCode", res.StatusCode, "Status", res.Status, "Body", body)
	return nil
}

func (r *AgentReconciler) initAgentName(url string, aName string, logger logr.Logger) error {
	post, err := http.NewRequest("POST", "http://"+url+"/", strings.NewReader(aName))
	if err != nil {
		return err
	}
	post.Header = http.Header{"Content-Type": []string{"text/plain"}}
	res, err := r.httpClient.Do(post)
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(res.Body)
	body := buf.String()
	logger.Info("Response after init client",
		"StatusCode", res.StatusCode, "Status", res.Status, "Body", body)
	return nil
}

func agentHealth(agent *matrixv1beta1.Agent, agentName string) int32 {
	aName := matrixv1beta1.AgentName(agentName)
	for _, a := range agent.Status.Agents {
		if a.Name == aName {
			return a.Health
		}
	}
	for _, a := range agent.Spec.Agents {
		if a.Name == aName {
			return *a.Health
		}
	}
	return 0
}

func agentName(agents []matrixv1beta1.SingleAgent, podName string) string {
	pos := strings.LastIndex(podName, "-")
	if pos == -1 {
		return ""
	}
	adjustedPos := pos + len("-")
	if adjustedPos >= len(podName) {
		return ""
	}
	pi, err := strconv.Atoi(podName[adjustedPos:])
	if err != nil {
		return ""
	}
	return string(agents[pi].Name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&matrixv1beta1.Agent{}).
		Owns(&appv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Watches(&source.Kind{Type: &corev1.Pod{}}, handler.EnqueueRequestsFromMapFunc(r.podUpdates)).
		Complete(r)
}
