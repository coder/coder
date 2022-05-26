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

export interface NetstatPort {
  name: string
  port: number
}

export interface NetstatResponse {
  readonly ports?: NetstatPort[]
  readonly error?: string
  readonly took?: number
}
