package codersdk

type HealthSettings struct {
	DismissedHealthchecks []string `json:"dismissed_healthchecks"`
}

type UpdateHealthSettings struct {
	DismissedHealthchecks []string `json:"dismissed_healthchecks"`
}
