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

// Operations
const (
	Create    rbac.Operation = "create"     // create a new resource
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
			APIKeys: {Create, ReadAll, UpdateAll, DeleteAll},
		},
		SiteAuditor: {
			AuditLogs: {ReadAll},
		},
		SiteManager: {
			AuditLogs:           {ReadAll},
			Configs:             {ReadAll, UpdateAll},
			SecretConfigs:       {ReadAll, UpdateAll},
			Workspaces:          {Create, ReadAll, UpdateAll, DeleteAll},
			Extensions:          {Create, DeleteAll},
			FeatureFlags:        {ReadAll, UpdateAll},
			ImageTags:           {Create, ReadAll, UpdateAll, DeleteAll},
			Images:              {Create, ReadAll, UpdateAll, DeleteAll},
			Metrics:             {ReadAll},
			OAuth:               {ReadAll, UpdateAll},
			OrganizationMembers: {Create, ReadAll, UpdateAll, DeleteAll},
			Organizations:       {Create, ReadAll, UpdateAll, DeleteAll},
			Registries:          {Create, ReadAll, UpdateAll, DeleteAll},
			ResourcePools:       {UpdateAll, ReadAll},
			Satellites:          {Create, ReadAll, UpdateAll, DeleteAll},
			SystemBanners:       {Create, ReadAll, UpdateAll, DeleteAll},
			TLSCertificates:     {Create, ReadAll, UpdateAll, DeleteAll},
			Usage:               {Create, ReadAll, UpdateAll, DeleteAll},
			Users:               {Create, ReadAll, UpdateAll, DeleteAll},
			WorkspaceProviders:  {Create, ReadAll, UpdateAll, DeleteAll},
		},
		SiteMember: {
			APIKeys:       {Create, ReadOwn, UpdateOwn, DeleteOwn},
			Configs:       {ReadAll},
			DevURLs:       {Create, ReadOwn, UpdateOwn, DeleteOwn},
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
			Workspaces:          {Create, ReadAll, UpdateAll, DeleteAll},
			ImageTags:           {Create, ReadAll, UpdateAll, DeleteAll},
			Images:              {Create, ReadAll, UpdateAll, DeleteAll},
			Metrics:             {ReadAll},
			OrganizationMembers: {Create, ReadAll, UpdateAll, DeleteAll},
			Organizations:       {ReadAll},
			Registries:          {Create, ReadAll, UpdateAll, DeleteAll},
			Users:               {ReadAll},
		},
		OrganizationMember: {
			DevURLs:             {ReadAll},
			Workspaces:          {Create, ReadAll, UpdateOwn, DeleteOwn},
			ImageTags:           {Create, ReadAll},
			Images:              {Create, ReadAll},
			Metrics:             {ReadOwn},
			OrganizationMembers: {},
			Organizations:       {},
			Registries:          {ReadAll},
			SystemBanners:       {ReadAll},
			Users:               {ReadOwn},
		},
		OrganizationRegistryManager: {
			Registries: {Create, ReadAll, UpdateAll, DeleteAll},
		},
	},
}
