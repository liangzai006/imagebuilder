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
	"encoding/json"
	"github.com/containerd/containerd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strings"

	dockerclient "github.com/docker/docker/client"
	imagebuilderv1 "imagebuilder/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ImageBuilderReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	ContainerdClient *containerd.Client
	DockerClient     *dockerclient.Client
	Type             string
	NodeName         string
}

func (r *ImageBuilderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	imageBuilder := &imagebuilderv1.ImageBuilder{}
	err := r.Client.Get(ctx, req.NamespacedName, imageBuilder)
	err = client.IgnoreNotFound(err)
	if err != nil {
		return ctrl.Result{}, err
	}
	if imageBuilder.Status.State == "Succeeded" || imageBuilder.Status.State == "Failed" {
		return ctrl.Result{}, nil
	}
	pod := corev1.Pod{}
	key := client.ObjectKey{Name: imageBuilder.Spec.PodName, Namespace: imageBuilder.Spec.Namespace}
	klog.Infof("get pod %s/%s", imageBuilder.Spec.Namespace, imageBuilder.Spec.PodName)
	err = r.Client.Get(ctx, key, &pod)
	if err != nil {
		klog.Warningf("get pod: %s/%s error: %v", imageBuilder.Spec.Namespace, imageBuilder.Spec.PodName, err)
		return ctrl.Result{}, nil
	}

	if pod.Spec.NodeName != r.NodeName {
		return ctrl.Result{}, nil
	}

	if imageBuilder.Status.State == "" {
		imageBuilder.Status.State = "Running"
		patch, _ := json.Marshal(imageBuilder)
		err = r.Status().Patch(ctx, imageBuilder, client.RawPatch(client.Merge.Type(), patch))
		if err != nil {
			klog.Errorf("update status error: %v", err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	var containerID string
	for _, i := range pod.Status.ContainerStatuses {
		if i.Name == imageBuilder.Spec.ContainerName {
			containerID = strings.Split(i.ContainerID, "://")[1]
		}
	}
	if containerID == "" {
		klog.Errorf("containerID is empty")
		return ctrl.Result{}, r.updateStatusFailed(ctx, imageBuilder, "containerID is empty")
	}

	to := imageBuilder.Spec.To
	if r.Type == "containerd" {
		err = r.containerdCommit(containerID, to)
		if err != nil {
			klog.Errorf("containerd commit error: %v", err)
			return ctrl.Result{}, r.updateStatusFailed(ctx, imageBuilder, err.Error())
		}
		klog.Infof("containerd commit success: %s", to)
		err = r.containerdPush(to, imageBuilder.Spec.Username, imageBuilder.Spec.Password)
		if err != nil {
			klog.Errorf("containerd push error: %v", err)
			return ctrl.Result{}, r.updateStatusFailed(ctx, imageBuilder, err.Error())
		}
		return r.updateStatusSuccess(ctx, imageBuilder, err)
	}

	if r.Type == "docker" {
		err = r.dockerCommit(containerID, to)
		if err != nil {
			klog.Errorf("docker commit error: %v", err)
			return ctrl.Result{}, r.updateStatusFailed(ctx, imageBuilder, err.Error())
		}
		klog.Infof("docker commit success: %s", to)
		err = r.dockerPush(to, imageBuilder.Spec.Username, imageBuilder.Spec.Password)
		if err != nil {
			klog.Errorf("docker push error: %v", err)
			return ctrl.Result{}, r.updateStatusFailed(ctx, imageBuilder, err.Error())
		}
		klog.Infof("docker push success: %s", to)
		return r.updateStatusSuccess(ctx, imageBuilder, err)
	}

	return ctrl.Result{}, nil
}

func (r *ImageBuilderReconciler) updateStatusSuccess(ctx context.Context, imageBuilder *imagebuilderv1.ImageBuilder, err error) (ctrl.Result, error) {
	imageBuilderNew := &imagebuilderv1.ImageBuilder{}
	imageBuilderNew.Name = imageBuilder.Name
	imageBuilderNew.Namespace = imageBuilder.Namespace
	imageBuilderNew.Status.State = "Succeeded"
	patch, _ := json.Marshal(imageBuilderNew)
	err = r.Status().Patch(ctx, imageBuilderNew, client.RawPatch(client.Merge.Type(), patch))
	return ctrl.Result{}, err
}

func (r *ImageBuilderReconciler) updateStatusFailed(ctx context.Context, imageBuilder *imagebuilderv1.ImageBuilder, reason string) error {
	imageBuilderNew := &imagebuilderv1.ImageBuilder{}
	imageBuilderNew.Name = imageBuilder.Name
	imageBuilderNew.Namespace = imageBuilder.Namespace
	imageBuilderNew.Status.State = "Failed"
	imageBuilderNew.Status.Reason = reason
	patch, _ := json.Marshal(imageBuilderNew)
	err := r.Status().Patch(ctx, imageBuilderNew, client.RawPatch(client.Merge.Type(), patch))
	return err
}

func (r *ImageBuilderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imagebuilderv1.ImageBuilder{}).
		Complete(r)
}

func NewImageBuilderReconciler(mgr manager.Manager) *ImageBuilderReconciler {
	nodeName := os.Getenv("NODE_NAME")
	k8sClient := kubernetes.NewForConfigOrDie(mgr.GetConfig())
	node, err := k8sClient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Fatal(err)
	}

	builder := &ImageBuilderReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		NodeName: nodeName,
	}
	klog.Infof("node %s ContainerRuntimeVersion: %s", nodeName, node.Status.NodeInfo.ContainerRuntimeVersion)
	//node.Status.NodeInfo.ContainerRuntimeVersion = "containerd"
	if strings.HasPrefix(node.Status.NodeInfo.ContainerRuntimeVersion, "docker") {
		cli, err := dockerclient.NewClientWithOpts(dockerclient.WithHost("unix:///var/run/docker.sock"))
		if err != nil {
			klog.Fatal(err)
		}
		builder.DockerClient = cli
		builder.Type = "docker"
		return builder
	}

	cdClient, err := containerd.New("/run/containerd/containerd.sock", containerd.WithDefaultNamespace("k8s.io"))
	if err != nil {
		klog.Fatal(err)
	}
	builder.ContainerdClient = cdClient
	builder.Type = "containerd"
	return builder
}
