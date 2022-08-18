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

export type EntitlementLevel = "entitled" | "grace_period" | "not_entitled"

export type LicenseFeatures = Record<
  string,
  {
    entitled: EntitlementLevel
    enabled: boolean
    limit?: number
    actual?: number
  }
>

export type Entitlements = {
  features: LicenseFeatures
  warnings: string[]
  has_license: boolean
}
