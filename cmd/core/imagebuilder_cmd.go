package core

import (
	"github.com/spf13/cobra"
	imagebuilderv1 "imagebuilder/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(imagebuilderv1.AddToScheme(scheme))
}

func NewImageBuilderCommand() *cobra.Command {
	builderCmd := &cobra.Command{
		Use:   "ib",
		Short: "save container instance to image",
	}

	builderCmd.AddCommand(NewControllerCommand())
	builderCmd.AddCommand(NewJobCommand())
	return builderCmd
}
