import * as TypesGen from "./typesGenerated"

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

// The generated type for ProvisionerJobLog is different than the one returned
// by the API.
export interface ProvisionerJobLog extends TypesGen.ProvisionerJobLog {
  readonly source: string
  readonly level: string
}
