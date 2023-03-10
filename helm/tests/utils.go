package tests

// This file contains utility functions for testing the Helm chart.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jinzhu/copier"
	"golang.org/x/xerrors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// renderManifests renders the chart with the given values and returns a map of
// rendered files and their contents.
func renderManifests(
	c *chart.Chart,
	orig *Values,
	mut func(*Values),
	options *chartutil.ReleaseOptions,
	capabilities *chartutil.Capabilities,
) (map[string]string, error) {
	var opts chartutil.ReleaseOptions
	if options == nil {
		opts = chartutil.ReleaseOptions{
			Name:      "coder",
			Namespace: "coder",
			Revision:  1,
			IsInstall: true,
			IsUpgrade: false,
		}
	} else {
		opts = *options
	}

	if capabilities == nil {
		capabilities = chartutil.DefaultCapabilities.Copy()
	}

	values := orig
	if mut != nil {
		values = &Values{}
		err := copier.CopyWithOption(values, orig, copier.Option{
			DeepCopy: true,
		})
		if err != nil {
			return nil, xerrors.Errorf("failed to deep copy values: %w", err)
		}

		mut(values)
	}

	valsMap, err := valuesToMap(values)
	if err != nil {
		return nil, xerrors.Errorf("failed to convert values to map: %w", err)
	}

	renderVals, err := chartutil.ToRenderValues(c, valsMap, opts, capabilities)
	if err != nil {
		return nil, xerrors.Errorf("failed to create render values: %w", err)
	}
	manifests, err := engine.Render(c, renderVals)
	if err != nil {
		return nil, xerrors.Errorf("failed to render chart: %w", err)
	}
	return manifests, nil
}

// loadObjectsFromManifests takes a map of rendered files and their contents
// and returns a slice of Kubernetes objects.
func loadObjectsFromManifests(manifests map[string]string) ([]runtime.Object, error) {
	deserializer := newDeserializer()

	var objs []runtime.Object

	// Helm returns a map of rendered files and contents
	for file, manifest := range manifests {
		reader := yaml.NewYAMLReader(bufio.NewReader(strings.NewReader(manifest)))
		// Split files into individual document chunks, then pass through
		// the deserializer
		for {
			document, err := reader.Read()
			if err != nil {
				// If we get an EOF, we've finished processing this file
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, err
			}

			// Exit the inner loop if we encounter an EOF
			if document == nil {
				break
			}

			// Skip empty documents
			if document[0] == '\n' {
				continue
			}

			obj, _, err := deserializer.Decode(document, nil, nil)
			if err != nil {
				return nil, xerrors.Errorf("error deserializing %q: %w", file, err)
			}

			objs = append(objs, obj)
		}
	}

	return objs, nil
}

// newDeserializer returns a new runtime.Decoder that can decode Kubernetes objects.
func newDeserializer() runtime.Decoder {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add appsv1 scheme: %v", err))
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add corev1 scheme: %v", err))
	}
	if err := networkingv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add networkingv1 scheme: %v", err))
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add rbacv1 scheme: %v", err))
	}
	deserializer := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	return deserializer
}

// mapToValues converts a map to a Values struct.
func mapToValues(m map[string]interface{}) (*Values, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(m); err != nil {
		return nil, err
	}
	var v Values
	if err := json.NewDecoder(&buf).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

// valuesToMap converts a Values struct to a map.
func valuesToMap(v *Values) (map[string]interface{}, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.NewDecoder(&buf).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}
