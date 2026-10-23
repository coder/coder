package provisionersdk

import (
	"maps"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/apiversion"
)

const (
	TagScope = "scope"
	TagOwner = "owner"

	// TagNetworkPolicy opts upgraded provisioners into native policy jobs.
	TagNetworkPolicy = "coder.network_policy"

	// TagNetworkPolicyDestroy requires native policy destroy support (API v1.22).
	TagNetworkPolicyDestroy = "coder.network_policy_destroy"

	// TagTemplateResources opts native workers into template resource jobs.
	TagTemplateResources = "coder.template_resources"

	// TemplateResourcesCapabilityVersion identifies native template job support.
	TemplateResourcesCapabilityVersion = "1"

	ScopeUser         = "user"
	ScopeOrganization = "organization"
)

// NetworkPolicyProvisionerTags returns a copy of tags with destroy capability
// derived from the worker's API version. Only workers already opted into native
// policy jobs receive it; configured tags cannot grant it to older workers.
func NetworkPolicyProvisionerTags(tags map[string]string, apiVersion string) map[string]string {
	tags = maps.Clone(tags)
	delete(tags, TagNetworkPolicyDestroy)
	major, minor, err := apiversion.Parse(apiVersion)
	if err == nil && major == 1 && minor >= 22 && tags[TagNetworkPolicy] == "1" {
		tags[TagNetworkPolicyDestroy] = "1"
	}
	return tags
}

// TemplateResourcesProvisionerTags strips template resource capability from
// workers using a protocol that cannot execute template resource jobs. An API
// version alone never opts a worker in.
func TemplateResourcesProvisionerTags(tags map[string]string, apiVersion string) map[string]string {
	tags = maps.Clone(tags)
	major, minor, err := apiversion.Parse(apiVersion)
	if err != nil || major != 1 || minor < 23 || tags[TagTemplateResources] != TemplateResourcesCapabilityVersion {
		delete(tags, TagTemplateResources)
	}
	return tags
}

// MutateTags adjusts the "owner" tag dependent on the "scope".
// If the scope is "user", the "owner" is changed to the user ID.
// This is for user-scoped provisioner daemons, where users should
// own their own operations.
// Multiple sets of tags may be passed to this function; they will
// be merged into one single tag set.
// Otherwise, the "owner" tag is always an empty string.
// NOTE: "owner" must NEVER be nil. Otherwise it will end up being
// duplicated in the database, as idx_provisioner_daemons_name_owner_key
// is a partial unique index that includes a JSON field.
func MutateTags(userID uuid.UUID, provided ...map[string]string) map[string]string {
	tags := map[string]string{}
	for _, m := range provided {
		tags = mergeTags(tags, m)
	}
	_, ok := tags[TagScope]
	if !ok {
		tags[TagScope] = ScopeOrganization
		tags[TagOwner] = ""
	}
	switch tags[TagScope] {
	case ScopeUser:
		tags[TagOwner] = userID.String()
	case ScopeOrganization:
		tags[TagOwner] = ""
	default:
		tags[TagScope] = ScopeOrganization
		tags[TagOwner] = ""
	}
	return tags
}

// mergeTags merges two sets of provisioner tags.
// If b[key] is an empty string, the value from a[key] is retained.
// This function handles nil maps gracefully.
func mergeTags(a, b map[string]string) map[string]string {
	m := make(map[string]string)
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		if v == "" {
			continue
		}
		m[k] = v
	}
	return m
}
