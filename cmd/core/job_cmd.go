package core

import (
	"context"
	"fmt"
	"github.com/containerd/containerd"
	dockerclient "github.com/docker/docker/client"
	"github.com/spf13/cobra"
	imagebuilderv1 "imagebuilder/api/v1"
	"imagebuilder/pkg/core"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"strings"
)

type JobOptions struct {
	Name        string
	Namespace   string
	ContainerId string
	client.Client
}

func newJobOptions() *JobOptions {
	return &JobOptions{}
}

func NewJobCommand() *cobra.Command {
	options := newJobOptions()
	jobCmd := &cobra.Command{
		Use: "job",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.validate(); err != nil {
				return err
			}
			clientConfig, err := config.GetConfig()
			if err != nil {
				klog.Error(err, "unable to get kubeconfig")
				clientConfig, err = rest.InClusterConfig()
				if err != nil {
					klog.Error(err, "unable to get InClusterConfig")
					return err
				}
			}

			r, err := client.New(clientConfig, client.Options{
				Scheme: scheme,
			})
			if err != nil {
				return err
			}
			options.Client = r
			imageBuilder := &imagebuilderv1.ImageBuilder{}
			err = r.Get(cmd.Context(), client.ObjectKey{Namespace: options.Namespace, Name: options.Name}, imageBuilder)
			if err != nil {
				return err
			}

			builderAction, err := options.initMontSock(cmd.Context(), imageBuilder.Status.Node)
			if err != nil {
				klog.Errorf("init mount sock error:%s", err)
				return err
			}

			if options.ContainerId == "" {
				klog.Errorf("containerID is empty")
				return err
			}

			to := imageBuilder.Spec.To
			err = builderAction.Commit(cmd.Context(), options.ContainerId, to)
			if err != nil {
				klog.Errorf("containerd commit error: %v", err)
				return err
			}
			klog.Infof("containerd commit success: %s", to)
			err = builderAction.Push(cmd.Context(), to, imageBuilder.Spec.Username, imageBuilder.Spec.Password)
			if err != nil {
				klog.Errorf("containerd push error: %v", err)
				return err
			}
			return nil

		},
	}

	options.AddCommandFlag(jobCmd)

	return jobCmd
}

func (j *JobOptions) AddCommandFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&j.Name, "name", "", "")
	cmd.Flags().StringVar(&j.Namespace, "namespace", "default", "")
	cmd.Flags().StringVar(&j.ContainerId, "container-id", "", "")
}

func (j *JobOptions) validate() error {
	if j.Name == "" {
		return fmt.Errorf("name is empty")
	}
	return nil
}

func (j *JobOptions) initMontSock(ctx context.Context, nodeName string) (core.ImageBuilderAction, error) {
	node := &corev1.Node{}
	err := j.Client.Get(ctx, client.ObjectKey{Name: nodeName}, node)
	klog.Infof("node %s ContainerRuntimeVersion: %s", nodeName, node.Status.NodeInfo.ContainerRuntimeVersion)
	if err != nil {
		klog.Fatal(err)
		return nil, err
	}
	containerRuntime := strings.Split(node.Status.NodeInfo.ContainerRuntimeVersion, "://")[0]
	switch containerRuntime {
	case "docker":
		cli, err := dockerclient.NewClientWithOpts(dockerclient.WithHost("unix:///var/run/docker.sock"))
		if err != nil {
			klog.Fatal(err)
			return nil, err
		}
		return &core.Docker{DockerClient: cli}, nil
	case "containerd":
		cdClient, err := containerd.New("/run/containerd/containerd.sock", containerd.WithDefaultNamespace("k8s.io"))
		if err != nil {
			klog.Fatal(err)
			return nil, err
		}
		return &core.Containerd{ContainerdClient: cdClient}, nil
	default:
		return nil, fmt.Errorf("unknown containerd runtime %s", containerRuntime)
	}

}
