// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package function provides the command to run the KRM function.
package function

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"

	"github.com/google/k8s-digester/pkg/logging"
	"github.com/google/k8s-digester/pkg/resolve"
)

// Cmd creates the KRM function command. This is the root command.
func Cmd(ctx context.Context) *cobra.Command {
	log := logging.CreateStdLogger("digester")
	resourceFn := createResourceFn(ctx, log)
	cmd := command.Build(framework.ResourceListProcessorFunc(resourceFn), command.StandaloneDisabled, false)
	customizeCmd(cmd)
	return cmd
}

// createResourceFn returns a function that iterates over the items in the
// resource list.
func createResourceFn(ctx context.Context, log logr.Logger) framework.ResourceListProcessorFunc {
	return func(resourceList *framework.ResourceList) error {
		log.V(2).Info("kubeconfig", "kubeconfig", viper.GetString("kubeconfig"))
		log.V(2).Info("offline", "offline", viper.GetBool("offline"))
		var config *rest.Config
		if !viper.GetBool("offline") {
			var kubeconfig string
			var err error
			kubeconfigs := strings.FieldsFunc(viper.GetString("kubeconfig"), func(r rune) bool {
				return r == ':' || r == ';'
			})
			if len(kubeconfigs) > 0 {
				kubeconfig = kubeconfigs[0]
			}
			
			config, err = createConfig(log, kubeconfig)
			if err != nil {
				return fmt.Errorf("could not create k8s client config: %w", err)
			}
		}
		for _, r := range resourceList.Items {
			if err := resolve.ImageTags(ctx, log, config, r); err != nil {
				return err
			}
		}
		return nil
	}
}

// customizeCmd modifies the kyaml function framework command by adding flags
// that this KRM function needs, and to make it more user-friendly.
func customizeCmd(cmd *cobra.Command) {
	cmd.Use = "digester"
	cmd.Short = "Resolve container image tags to digests"
	cmd.Long = "Digester adds digests to container and " +
		"init container images in Kubernetes pod and pod template " +
		"specs.\n\nUse either as a mutating admission webhook, " +
		"or as a client-side KRM function with kpt or kustomize."
	cmd.Flags().String("kubeconfig", getKubeconfigDefault(),
		"(optional) absolute path to the kubeconfig file. Requires offline=false.")
	viper.BindPFlag("kubeconfig", cmd.Flags().Lookup("kubeconfig"))
	viper.BindEnv("kubeconfig")
	cmd.Flags().Bool("offline", true,
		"do not connect to Kubernetes API server to retrieve imagePullSecrets")
	viper.BindPFlag("offline", cmd.Flags().Lookup("offline"))
	viper.BindEnv("offline")
}

// getKubeconfigDefault determines the default value of the --kubeconfig flag.
func getKubeconfigDefault() string {
	var kubeconfigDefault string
	home := homedir.HomeDir()
	if home != "" {
		kubeconfigHomePath := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kubeconfigHomePath); err == nil {
			kubeconfigDefault = kubeconfigHomePath
		}
	}
	return kubeconfigDefault
}

// createConfig creates a k8s client config using either in-cluster config
// or the provided kubeconfig file.
func createConfig(log logr.Logger, kubeconfig string) (*rest.Config, error) {
	if kubeconfig == "" {
		log.V(1).Info("using in-cluster config")
		return rest.InClusterConfig()
	}
	log.V(1).Info("using kubeconfig file", "kubeconfig", kubeconfig)
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
