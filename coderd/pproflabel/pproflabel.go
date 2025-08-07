package pproflabel

import (
	"context"
	"runtime/pprof"
)

// Go is just a convince wrapper to set off a labeled goroutine.
func Go(ctx context.Context, labels pprof.LabelSet, f func(context.Context)) {
	go pprof.Do(ctx, labels, f)
}

func Do(ctx context.Context, labels pprof.LabelSet, f func(context.Context)) {
	pprof.Do(ctx, labels, f)
}

const (
	// ServiceTag should not collide with the pyroscope built-in tag "service".
	// Use `coder_` to avoid collisions.
	ServiceTag = "coder_service"

	ServiceHTTPServer           = "http-api"
	ServiceLifecycles           = "lifecycle-executor"
	ServiceMetricCollector      = "metrics-collector"
	ServicePrebuildReconciler   = "prebuilds-reconciler"
	ServiceTerraformProvisioner = "terraform-provisioner"
)

func Service(name string, pairs ...string) pprof.LabelSet {
	return pprof.Labels(append([]string{ServiceTag, name}, pairs...)...)
}
