package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/converter"
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
	if !options.AllPlatforms {
		pushRef = ref + "-tmp-reduced-platform"
		// Push fails with "400 Bad Request" when the manifest is multi-platform but we do not locally have multi-platform blobs.
		// So we create a tmp reduced-platform image to avoid the error.
		platImg, err := converter.Convert(ctx, r.ContainerdClient, pushRef, ref, converter.WithPlatform(platMC))
		if err != nil {
			if len(options.Platforms) == 0 {
				return fmt.Errorf("failed to create a tmp single-platform image %q: %w", pushRef, err)
			}
			return fmt.Errorf("failed to create a tmp reduced-platform image %q (platform=%v): %w", pushRef, options.Platforms, err)
		}
		defer r.ContainerdClient.ImageService().Delete(ctx, platImg.Name, images.SynchronousDelete())
		klog.Infof("pushing as a reduced-platform image (%s, %s)", platImg.Target.MediaType, platImg.Target.Digest)
	}

	// In order to push images where most layers are the same but the
	// repository name is different, it is necessary to refresh the
	// PushTracker. Otherwise, the MANIFEST_BLOB_UNKNOWN error will occur due
	// to the registry not creating the corresponding layer link file,
	// resulting in the failure of the entire image push.
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
	ho.DefaultTLS = nil
	return &ho, nil
}
