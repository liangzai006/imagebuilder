package core

import (
	"github.com/spf13/cobra"
	"imagebuilder/pkg/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type ControllerOptions struct {
}

func NewControllerOptions() *ControllerOptions {
	return &ControllerOptions{}
}

func NewControllerCommand() *cobra.Command {
	c := NewControllerOptions()
	cmd := &cobra.Command{
		Use: "controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
				Scheme:  scheme,
				Metrics: metricsserver.Options{BindAddress: "0"},
			})
			if err != nil {
				klog.Fatalf("unable to create manager: %v", err)
				return err
			}
			clientSet, err := kubernetes.NewForConfig(mgr.GetConfig())
			if err != nil {
				klog.Errorf("failed to start manager")
				return err
			}
			buildNamespaces := os.Getenv("imagebuild_namespace")
			buildName := os.Getenv("imagebuild_name")
			klog.Infof("get controlelr pod resources %s/%s", buildName, buildNamespaces)
			pod, err := clientSet.CoreV1().Pods(buildNamespaces).Get(cmd.Context(), buildName, metav1.GetOptions{})
			if err != nil {
				klog.Errorf("controller pod %s/%s not found. err%s", buildName, buildNamespaces, err)
				//return err
			}
			if err = (&controller.ImageBuilderReconciler{
				Client:     mgr.GetClient(),
				Scheme:     mgr.GetScheme(),
				ClientSet:  clientSet,
				ManagerPod: pod,
			}).SetupWithManager(mgr); err != nil {
				klog.Fatalf("unable to create manager: %v", err)
				return err
			}
			klog.Info("starting manager")
			if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
				klog.Fatalf("problem running manager: %v", err)
				return err
			}
			return nil
		},
	}
	c.addCommandFlag(cmd)
	return cmd
}

func (c *ControllerOptions) addCommandFlag(cmd *cobra.Command) {

}
