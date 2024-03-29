package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/containerd/containerd/reference"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	dockerconfig "github.com/containerd/containerd/remotes/docker/config"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/containerd/nerdctl/pkg/imgutil/push"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/signutil"
	"k8s.io/klog/v2"
	"os"
)

func (r *ImageBuilderReconciler) containerdCommit(containerID, to string) error {
	options := types.ContainerCommitOptions{
		Stdout: os.Stdout,
	}
	err := container.Commit(context.TODO(), r.ContainerdClient, to, containerID, options)
	if err != nil {
		klog.Errorf("containerdCommit error: %v", err)
		return err
	}
	klog.Infof("containerdCommit success: %v", to)
	return err
}

func (r *ImageBuilderReconciler) containerdPush(rawRef, Username, Password string) error {
	ctx := context.TODO()
	options := types.ImagePushOptions{
		Stdout: os.Stdout,
	}
	options.GOptions.InsecureRegistry = true
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return err
	}
	ref := named.String()

	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platforms)
	if err != nil {
		return err
	}
	pushRef := ref

	pushTracker := docker.NewInMemoryTracker()

	pushFunc := func(remote remotes.Resolver) error {
		return push.Push(ctx, r.ContainerdClient, remote, pushTracker, options.Stdout, pushRef, ref, platMC, options.AllowNondistributableArtifacts, options.Quiet)
	}
	ho, err := NewHostOptions(Username, Password)
	if err != nil {
		return err
	}
	resolverOpts := docker.ResolverOptions{
		Tracker: pushTracker,
		Hosts:   dockerconfig.ConfigureHosts(ctx, *ho),
	}

	resolver := docker.NewResolver(resolverOpts)
	err = pushFunc(resolver)
	if err != nil {
		klog.Errorf("containerdPush error: %v", err)
		return err
	}

	img, err := r.ContainerdClient.ImageService().Get(ctx, pushRef)
	if err != nil {
		return err
	}
	refSpec, err := reference.Parse(pushRef)
	if err != nil {
		return err
	}
	signRef := fmt.Sprintf("%s@%s", refSpec.String(), img.Target.Digest.String())
	if err = signutil.Sign(signRef, options.GOptions.Experimental, options.SignOptions); err != nil {
		return err
	}

	klog.Infof("containerdPush success: %s", named.Name())

	return nil
}

func NewHostOptions(Username, Password string) (*dockerconfig.HostOptions, error) {
	var ho dockerconfig.HostOptions
	if Username != "" {
		ho.Credentials = func(s string) (string, string, error) {
			klog.Infof("authCreds: %s use Username %s, Password %s", s, Username, Password)
			return Username, Password, nil
		}
	}
	ho.DefaultTLS = &tls.Config{
		InsecureSkipVerify: true,
	}
	ho.DefaultScheme = "http"
	//ho.DefaultTLS = nil
	return &ho, nil
}
