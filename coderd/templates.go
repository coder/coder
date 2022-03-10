package coderd

type Template struct {
	ID          string
	Name        string
	Description string

	ProjectVersionParameterSchema []ProjectVersionParameterSchema `json:"schema"`
	Resources                     []WorkspaceResource             `json:"resources"`
}
