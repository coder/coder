package catalog

import "fmt"

const (
	CDevLabel   = "cdev"
	CDevService = "cdev/service"
)

type ServiceName string

const (
	CDevBuildSlim ServiceName = "build-slim"
)

type Labels map[string]string

func NewLabels(service ServiceName) Labels {
	return map[string]string{
		CDevLabel:   "true",
		CDevService: string(service),
	}
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
