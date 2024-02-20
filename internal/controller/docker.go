package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/docker/docker/api/types"
	"io"
	"os"
)

func (r *ImageBuilderReconciler) dockerCommit(containerID, to string) error {

	opts := types.ContainerCommitOptions{
		Reference: to,
	}
	_, err := r.DockerClient.ContainerCommit(context.Background(), containerID, opts)

	return err
}

type AuthConfig struct {
	Username string
	Password string
}

func (r *ImageBuilderReconciler) dockerPush(imageName, Username, Password string) error {

	authConfig := AuthConfig{Username: Username, Password: Password}
	authBytes, _ := json.Marshal(authConfig)
	encodedAuth := base64.StdEncoding.EncodeToString(authBytes)

	var opts types.ImagePushOptions
	if Username != "" {
		opts.RegistryAuth = encodedAuth
	}

	out, err := r.DockerClient.ImagePush(context.Background(), imageName, opts)
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
