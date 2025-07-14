package codersdk

// TODO:
type CreateUserSecretRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value,omitempty"`

	Name       string    `json:"name,omitempty" validate:"omitempty,template_version_name"`
	Message    string    `json:"message,omitempty" validate:"lt=1048577"`
	TemplateID uuid.UUID `json:"template_id,omitempty" format:"uuid"`
}
