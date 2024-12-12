package core

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"io"
	"os"
	"strings"
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

	scanner := bufio.NewScanner(out)
	//{"errorDetail":{"message":"received unexpected HTTP status: 504 Gateway Time-out"},"error":"received unexpected HTTP status: 504 Gateway Time-out"}

	for scanner.Scan() {
		str := scanner.Text()
		fmt.Println(str)
		if strings.Contains(str, "error") {
			m := map[string]interface{}{}
			err = json.Unmarshal(scanner.Bytes(), &m)
			if err == nil {
				if v, ok := m["error"]; ok {
					return errors.New(v.(string))
				}

			}
		}

	}

	//_, err = io.Copy(os.Stdout, out)
	//if err != nil {
	//	return err
	//}
	defer out.Close()
	return err
}

func (r *Docker) Save(ctx context.Context, imageName, outputPath string) error {
	// Get the image in a tarball format
	reader, err := r.DockerClient.ImageSave(ctx, []string{imageName})
	if err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}
	defer reader.Close()

	// Create or open the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Copy the image data to the output file
	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to write image to file: %w", err)
	}

	fmt.Printf("Image saved to %s\n", outputPath)
	return nil
}
