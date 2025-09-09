package utils

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GetKubeConfig returns a Kubernetes configuration from the default kubeconfig file
func GetKubeConfig() (*rest.Config, error) {
	// Try to get kubeconfig from environment variable
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		// Use default kubeconfig location
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, NewFrameworkError(ErrCodeConfigNotFound,
				"failed to get user home directory", err)
		}
		kubeconfig = filepath.Join(homeDir, ".kube", "config")
	}

	// Load the kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, WrapError(err, ErrCodeConfigNotFound,
			"failed to load kubeconfig")
	}

	return config, nil
}

// GetKubeConfigFromBytes creates a Kubernetes config from byte data
func GetKubeConfigFromBytes(kubeconfigData []byte) (*rest.Config, error) {
	config, err := clientcmd.NewClientConfigFromBytes(kubeconfigData)
	if err != nil {
		return nil, WrapError(err, ErrCodeConfigParseFailed,
			"failed to parse kubeconfig from bytes")
	}

	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, WrapError(err, ErrCodeConfigParseFailed,
			"failed to create REST config from kubeconfig")
	}

	return restConfig, nil
}
