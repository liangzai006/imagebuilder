package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"io"
	"os"
)

type Docker struct {
	DockerClient *dockerclient.Client
}

func (r *Docker) Commit(ctx context.Context, containerID, to string) error {

	opts := types.ContainerCommitOptions{
		Reference: to,
		Pause:     true,
	}
	_, err := r.DockerClient.ContainerCommit(ctx, containerID, opts)

	return err
}

type AuthConfig struct {
	Username string
	Password string
}

func (r *Docker) Push(ctx context.Context, imageName, Username, Password string) error {

	authConfig := AuthConfig{Username: Username, Password: Password}
	authBytes, _ := json.Marshal(authConfig)
	encodedAuth := base64.StdEncoding.EncodeToString(authBytes)

	var opts types.ImagePushOptions
	if Username != "" {
		opts.RegistryAuth = encodedAuth
	}

	out, err := r.DockerClient.ImagePush(ctx, imageName, opts)
	if err != nil {
		return err
	}
	_, err = io.Copy(os.Stdout, out)
	if err != nil {
		return err
	}
	defer out.Close()
	return err
}
