package core

import (
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type JobOptions struct {
	Namespace     string
	Name          string
	JobNamespace  string
	ImageRegistry string
	ContainerId   string
	NodeName      string
}

func JobTemplate(o JobOptions) *v1.Job {

	privileged := true

	return &v1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.JobNamespace,
		},
		Spec: v1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"sidecar.istio.io/inject": "false"},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "imagebuilder-service-account",
					Containers: []corev1.Container{{
						Name:            "imagebuild-job",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args:            []string{"job", "--name", o.Name, "--namespace", o.Namespace, "--container-id", o.ContainerId},
						Image:           o.ImageRegistry,
						SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "docker-socket",
							MountPath: "/var/run/docker.sock",
						}, {
							Name:      "containerd-socket",
							MountPath: "/run/containerd/containerd.sock",
						},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("128m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					},
					},
					Volumes: []corev1.Volume{{
						Name: "docker-socket",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"},
						},
					}, {
						Name: "containerd-socket",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/run/containerd/containerd.sock"},
						},
					}},
					NodeName: o.NodeName,
					Tolerations: []corev1.Toleration{{
						Operator: corev1.TolerationOpExists,
					}},
				},
			},
		},
	}
}
