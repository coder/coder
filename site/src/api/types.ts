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

export type LicensePermission = "audit" | "createUser" | "createOrg" | "adminScheduling"

export type LicenseFeatures = Record<LicensePermission, {
  entitled: boolean
  enabled: boolean
  limit?: number
  actual?: number
}>

export type LicenseData = {
  features: LicenseFeatures
  warnings: string[]
}
