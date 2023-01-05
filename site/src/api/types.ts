export interface UserAgent {
  readonly browser: string
  readonly device: string
  readonly ip_address: string
  readonly os: string
}

export interface ReconnectingPTYRequest {
  readonly data?: string
  readonly height?: number
  readonly width?: number
}

export type WorkspaceBuildTransition = "start" | "stop" | "delete"

export type Message = { message: string }

// Keep up to date with coder/codersdk/features.go
export enum FeatureNames {
  AuditLog = "audit_log",
  UserLimit = "user_limit",
  BrowserOnly = "browser_only",
  SCIM = "scim",
  TemplateRBAC = "template_rbac",
  HighAvailability = "high_availability",
  Appearance = "appearance",
}
