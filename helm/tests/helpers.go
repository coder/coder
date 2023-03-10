package tests

// This file contains helper functions for the tests.

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// requireDeployment finds a deployment in the given slice of objects with the
// given name, or fails the test.
func requireDeployment(t testing.TB, objs []runtime.Object, name string) *appsv1.Deployment {
	names := []string{}
	for _, obj := range objs {
		if deployment, ok := obj.(*appsv1.Deployment); ok {
			if deployment.Name == name {
				return deployment
			}
			names = append(names, deployment.Name)
		}
	}

	t.Fatalf("failed to find deployment %q, found %v", name, names)
	return nil
}

func requireEnv(t testing.TB, envs []corev1.EnvVar, name, value string) {
	for _, env := range envs {
		if env.Name == name {
			require.Equal(t, value, env.Value, "unexpected value for env %q", name)
			return
		}
	}

	t.Fatalf("failed to find env %q", name)
}
