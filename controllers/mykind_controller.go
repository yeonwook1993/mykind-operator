/*
Copyright 2021.

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
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	webappv1alpha1 "yeonwook1993/mykind-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// MykindReconciler reconciles a Mykind object

type MykindReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=webapp.crd.tutorial.kubebuilder.io,resources=mykinds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=webapp.crd.tutorial.kubebuilder.io,resources=mykinds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=webapp.crd.tutorial.kubebuilder.io,resources=mykinds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Mykind object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *MykindReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("mykind", req.NamespacedName)
	log.Info("Reconciling Mykind")

	mykind := &webappv1alpha1.Mykind{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, mykind)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("My kind not found. Ignore since obj must be deleted.")
			return reconcile.Result{}, nil
		}
		log.Error(err, "Failed to get mykind")
		return reconcile.Result{}, err
	}

	deployment := &appsv1.Deployment{}

	//해당 deploy가 존재하는지 확인

	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mykind.Name,
		Namespace: mykind.Namespace,
	}, deployment)

	//동작은 실패했지만 에러가 아님 (myKind cr을 위한 deploy가 존재하지 않으므로 하나 만들어줌)
	if err != nil && errors.IsNotFound(err) {
		newDeployment := r.createDeployment(mykind)
		log.Info("Create new Deploy...")
		err = r.Client.Create(context.TODO(), newDeployment)
		if err != nil {
			log.Error(err, "Failed to create Deployment...")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Cant get Deployment... Namespace %s, Name %s", deployment.Namespace, deployment.Name)
		return reconcile.Result{}, err
	}

	//replica 확인해서 업데이트하기

	size := mykind.Spec.Size
	if *deployment.Spec.Replicas != size {
		deployment.Spec.Replicas = &size
		err = r.Client.Update(context.TODO(), deployment)
		if err != nil {
			log.Error(err, "Failed to update Deployment.", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
			return ctrl.Result{}, err
		}
	}

	podList := &corev1.PodList{}
	ls := labelsFormyKind(mykind.Name)

	listOps := []client.ListOption{
		client.InNamespace(req.NamespacedName.Namespace),
		client.MatchingLabels(ls),
	}
	err = r.Client.List(context.TODO(), podList, listOps...)
	if err != nil {
		log.Error(err, "Failed to list pods.", "mykind.Namespace", mykind.Namespace, "mykind.Name", mykind.Name)
		return ctrl.Result{}, err
	}
	podNames := getPodNames(podList.Items)

	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, mykind.Status.Nodes) {
		mykind.Status.Nodes = podNames
		err := r.Client.Status().Update(context.TODO(), mykind)
		if err != nil {
			log.Error(err, "Failed to update mykind status.")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MykindReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1alpha1.Mykind{}).
		Complete(r)
}

func labelsFormyKind(name string) map[string]string {
	return map[string]string{"app": "mykind", "mykind_cr": name}
}

func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

func (r *MykindReconciler) createDeployment(ins *webappv1alpha1.Mykind) *appsv1.Deployment {
	lb := labelsFormyKind("mykind")
	replicas := ins.Spec.Size
	dep := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      ins.Name,
			Namespace: ins.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: lb,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: lb,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:   "memcached:1.4.36-alpine",
						Name:    "memcached",
						Command: []string{"memcached", "-m=64", "-o", "modern", "-v"},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 11211,
							Name:          "memcached",
						}},
					}},
				},
			},
		},
	}
	ctrl.SetControllerReference(ins, dep, r.Scheme)
	return dep
}
