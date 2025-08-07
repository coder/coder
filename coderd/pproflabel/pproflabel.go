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
	SystemTag = "service"

	SystemHTTPServer           = "http-api"
	SystemLifecycles           = "lifecycle-executor"
	SystemMetricCollector      = "metrics-collector"
	SystemPrebuildReconciler   = "prebuilds-reconciler"
	SystemTerraformProvisioner = "terraform-provisioner"
)

func Service(name string, pairs ...string) pprof.LabelSet {
	return pprof.Labels(append([]string{SystemTag, name}, pairs...)...)
}
