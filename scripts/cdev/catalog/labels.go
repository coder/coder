package catalog

import "fmt"

const (
	CDevLabel   = "cdev"
	CDevService = "cdev/service"
)

type ServiceName string

const (
	CDevDocker      ServiceName = "docker"
	CDevBuildSlim   ServiceName = "build-slim"
	CDevPostgres    ServiceName = "postgres"
	CDevCoderd      ServiceName = "coderd"
	CDevOIDC        ServiceName = "oidc"
	CDevProvisioner ServiceName = "provisioner"
	CDevPrometheus  ServiceName = "prometheus"
	CDevSetup       ServiceName = "setup"
	CDevSite        ServiceName = "site"
)

type Labels map[string]string

func NewServiceLabels(service ServiceName) Labels {
	return NewLabels().WithService(service)
}

func NewLabels() Labels {
	return map[string]string{
		CDevLabel: "true",
	}
}

func (l Labels) WithService(service ServiceName) Labels {
	return l.With(CDevService, string(service))
}

func (l Labels) With(key, value string) Labels {
	l[key] = value
	return l
}

func (l Labels) Filter() map[string][]string {
	list := make([]string, 0)
	for k, v := range l {
		list = append(list, fmt.Sprintf("%s=%s", k, v))
	}

	return map[string][]string{
		"label": list,
	}
}
