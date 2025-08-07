package pproflabel

import (
	"context"
	"runtime/pprof"
)

// Go is just a convince wrapper to set off a labeled goroutine.
func Go(ctx context.Context, labels pprof.LabelSet, f func(context.Context)) {
	go pprof.Do(ctx, labels, f)
}

const (
	ServiceTerraformProvisioner = "terraform-provisioner"
	ServiceMetricCollector      = "metric-collector"
	ServiceLifecycles           = "lifecycles"
	ServiceHTTPServer           = "http-server"
	ServicePrebuildReconciler   = "prebuild-reconciler"
)

func Service(name string) pprof.LabelSet {
	return pprof.Labels("service", name)
}
