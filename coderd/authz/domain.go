package authz

import (
	"github.com/coder/coder/coderd/authz/rbac"
)

// Site Roles
const (
	System      rbac.Role = "system"
	SiteAdmin   rbac.Role = "site-admin"
	SiteAuditor rbac.Role = "site-auditor"
	SiteManager rbac.Role = "site-manager"
	SiteMember  rbac.Role = "site-member"
)

// Organization Roles
const (
	OrganizationAdmin           rbac.Role = "organization-admin"
	OrganizationManager         rbac.Role = "organization-manager"
	OrganizationMember          rbac.Role = "organization-member"
	OrganizationRegistryManager rbac.Role = "registry-manager"
)

// Resources
const (
	APIKeys   rbac.Resource = "api-keys"
	AuditLogs rbac.Resource = "audit-logs"
	Auth      rbac.Resource = "auth"
	// !!!! USE SecretConfigs WHEN DEALING WITH SECRET CONFIG STRUCTS !!!!
	Configs             rbac.Resource = "configs"
	SecretConfigs       rbac.Resource = "secret-configs"
	DevURLs             rbac.Resource = "dev-urls"
	Extensions          rbac.Resource = "extensions"
	FeatureFlags        rbac.Resource = "feature-flags"
	ImageTags           rbac.Resource = "image-tags"
	Images              rbac.Resource = "images"
	Licenses            rbac.Resource = "licenses"
	Metrics             rbac.Resource = "metrics"
	OAuth               rbac.Resource = "oauth"
	OrganizationMembers rbac.Resource = "organization-members"
	Organizations       rbac.Resource = "organizations"
	Registries          rbac.Resource = "registries"
	Satellites          rbac.Resource = "satellites"
	ResourcePools       rbac.Resource = "resource-pools"
	SystemBanners       rbac.Resource = "system-banners"
	TLSCertificates     rbac.Resource = "tls-certificates"
	Usage               rbac.Resource = "usage"
	Users               rbac.Resource = "users"
	WorkspaceProviders  rbac.Resource = "workspace-providers"
	Workspaces          rbac.Resource = "environments"
)

// Operations **must** have an -all or -own suffix
const (
	CreateOwn rbac.Operation = "create-own" // create a new resource owned by me
	CreateAll rbac.Operation = "create-all" // create a new resource in this scope
	DeleteAll rbac.Operation = "delete-all" // delete any of these resources in this scope
	DeleteOwn rbac.Operation = "delete-own" // delete any of these resources owned in this scope
	ReadAll   rbac.Operation = "read-all"   // read all fields in any of these resources in this scope
	ReadOwn   rbac.Operation = "read-own"   // read all fields in any of these resources owned in this scope
	UpdateAll rbac.Operation = "update-all" // update all fields of these resources in this scope
	UpdateOwn rbac.Operation = "update-own" // update all fields in any of these resources owned in this scope
)

// SiteEnforcer is a rbac.Enforcer scoped to the entire site.
var SiteEnforcer = rbac.Enforcer{
	Inheritances: rbac.Inheritances{
		// System receives privileges on an as-needed basis to reduce the attack
		// surface.
		System:      {},
		SiteAdmin:   {SiteManager, SiteMember},
		SiteAuditor: {SiteMember},
		SiteManager: {SiteMember},
	},
	RolePermissions: rbac.RolePermissions{
		System: {
			Configs:       {ReadAll},
			ResourcePools: {UpdateAll},
		},
		SiteAdmin: {
			APIKeys: {CreateAll, ReadAll, UpdateAll, DeleteAll},
		},
		SiteAuditor: {
			AuditLogs: {ReadAll},
		},
		SiteManager: {
			AuditLogs:           {ReadAll},
			Configs:             {ReadAll, UpdateAll},
			SecretConfigs:       {ReadAll, UpdateAll},
			Workspaces:          {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Extensions:          {CreateAll, DeleteAll},
			FeatureFlags:        {ReadAll, UpdateAll},
			ImageTags:           {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Images:              {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Metrics:             {ReadAll},
			OAuth:               {ReadAll, UpdateAll},
			OrganizationMembers: {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Organizations:       {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Registries:          {CreateAll, ReadAll, UpdateAll, DeleteAll},
			ResourcePools:       {UpdateAll, ReadAll},
			Satellites:          {CreateAll, ReadAll, UpdateAll, DeleteAll},
			SystemBanners:       {CreateAll, ReadAll, UpdateAll, DeleteAll},
			TLSCertificates:     {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Usage:               {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Users:               {CreateAll, ReadAll, UpdateAll, DeleteAll},
			WorkspaceProviders:  {CreateAll, ReadAll, UpdateAll, DeleteAll},
		},
		SiteMember: {
			APIKeys:       {CreateOwn, ReadOwn, UpdateOwn, DeleteOwn},
			Configs:       {ReadAll},
			DevURLs:       {CreateOwn, ReadOwn, UpdateOwn, DeleteOwn},
			FeatureFlags:  {ReadAll},
			ResourcePools: {ReadAll},
			Metrics:       {ReadOwn},
			Users:         {ReadOwn, UpdateOwn},
		},
	},
}

// OrganizationEnforcer is a rbac.Enforcer scoped to an organization.
var OrganizationEnforcer = rbac.Enforcer{
	Inheritances: rbac.Inheritances{
		OrganizationAdmin:   {OrganizationManager, OrganizationMember},
		OrganizationManager: {OrganizationMember},
	},
	RolePermissions: rbac.RolePermissions{
		OrganizationManager: {
			Workspaces:          {CreateAll, ReadAll, UpdateAll, DeleteAll},
			ImageTags:           {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Images:              {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Metrics:             {ReadAll},
			OrganizationMembers: {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Organizations:       {ReadAll},
			Registries:          {CreateAll, ReadAll, UpdateAll, DeleteAll},
			Users:               {ReadAll},
		},
		OrganizationMember: {
			DevURLs:             {ReadAll},
			Workspaces:          {CreateOwn, ReadAll, UpdateOwn, DeleteOwn},
			ImageTags:           {CreateOwn, ReadAll},
			Images:              {CreateOwn, ReadAll},
			Metrics:             {ReadOwn},
			OrganizationMembers: {},
			Organizations:       {},
			Registries:          {ReadAll},
			SystemBanners:       {ReadAll},
			Users:               {ReadOwn},
		},
		OrganizationRegistryManager: {
			Registries: {CreateAll, ReadAll, UpdateAll, DeleteAll},
		},
	},
}
