/*
Copyright 2024.

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

package controller

import (
	"context"
	imagebuilderv1 "imagebuilder/api/v1"
	"imagebuilder/pkg/constant"
	"imagebuilder/pkg/core"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

type ImageBuilderReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	ClientSet  *kubernetes.Clientset
	ManagerPod *corev1.Pod
}

func (r *ImageBuilderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	builder := &imagebuilderv1.ImageBuilder{}
	err := r.Client.Get(ctx, req.NamespacedName, builder)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if builder.DeletionTimestamp != nil {
		klog.Warningf("%s cr deleting", builder.Name)
		return ctrl.Result{}, nil
	}

	if builder.Spec.PodName == "" {
		klog.Errorf("cr podName is empty")
		builder.Status.State = constant.Failed
		builder.Status.Reason = "cr podName is empty"
		err = r.Status().Update(ctx, builder)
		if err != nil {
			klog.Errorf("update builder status error err:%s", err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if builder.Status.State == constant.Succeeded || builder.Status.State == constant.Failed {
		if builder.Status.State == constant.Succeeded {
			background := metav1.DeletePropagationBackground
			err = r.ClientSet.BatchV1().Jobs(r.ManagerPod.Namespace).Delete(ctx, builder.Name, metav1.DeleteOptions{PropagationPolicy: &background})
			if err != nil && !errors.IsNotFound(err) {
				klog.Error("delete job error\n", err, "name:", builder.Name, "namespace:", r.ManagerPod.Namespace)
			}
		}
		return ctrl.Result{}, nil
	}

	klog.Infof("get pod %s/%s", builder.Spec.Namespace, builder.Spec.PodName)
	pod, err := r.ClientSet.CoreV1().Pods(builder.Spec.Namespace).Get(ctx, builder.Spec.PodName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("get pod: %s/%s error: %v", builder.Spec.Namespace, builder.Spec.PodName, err)
		return ctrl.Result{}, nil
	}

	if builder.Status.State == "" {
		builder.Status.State = constant.Creating
		builder.Status.Node = pod.Spec.NodeName
		err = r.Status().Update(ctx, builder)
		if err != nil {
			klog.Errorf("update status error: %v", err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	m := core.JobOptions{
		Namespace:     builder.Namespace,
		Name:          builder.Name,
		JobNamespace:  r.ManagerPod.Namespace,
		ImageRegistry: r.ManagerPod.Spec.Containers[0].Image,
		NodeName:      builder.Status.Node,
	}

	for _, i := range pod.Status.ContainerStatuses {
		if i.Name == builder.Spec.ContainerName {
			m.ContainerId = strings.Split(i.ContainerID, "://")[1]
		}
	}

	klog.Infof("check for failed tasks.  %s/%s", m.Name, m.JobNamespace)
	_, err = r.ClientSet.BatchV1().Jobs(m.JobNamespace).Get(ctx, m.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Create(ctx, core.JobTemplate(m))
			if err != nil {
				klog.Errorf("failed to create builder job. err:%s", err)
				return ctrl.Result{}, err
			}
			go func(o core.JobOptions) {

				for {
					j, err := r.ClientSet.BatchV1().Jobs(o.JobNamespace).Get(ctx, o.Name, metav1.GetOptions{})
					if errors.IsNotFound(err) {
						break
					}
					klog.Infof("get job status for '%s/%s'. createtime:%s", o.Name, o.JobNamespace, j.CreationTimestamp.String())
					if (len(j.Status.Conditions) > 0 && j.Status.Conditions[0].Type == batchv1.JobFailed) || err != nil {
						builder.Status.State = constant.Failed
						if err != nil {
							builder.Status.Reason = err.Error()
						} else if j.Status.Conditions[0].Type == batchv1.JobFailed {
							builder.Status.Reason = j.Status.Conditions[0].Message
						}
						err = r.Status().Update(ctx, builder)
						if err != nil {
							klog.Errorf("update status error: %v", err)
						}
						break
					}
					time.Sleep(10 * time.Second)
				}
			}(m)
		}
	}

	return ctrl.Result{}, nil
}

func (r *ImageBuilderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imagebuilderv1.ImageBuilder{}).
		Complete(r)
}
