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

export interface DERPMap {
  readonly Regions: Record<number, DERPRegion>
}

export interface DERPRegion {
  readonly EmbeddedRelay: boolean
  readonly RegionID: number
  readonly RegionCode: string
  readonly RegionName: string
  readonly Avoid: boolean
  readonly Nodes: DERPNode[]
}

export interface DERPNode {
  readonly Name: string
  readonly RegionID: number
  readonly Hostname: string
  readonly CertName: string
  readonly IPv4: string
  readonly IPv6: string
  readonly STUNPort: number
  readonly STUNOnly: boolean
  readonly DERPPort: number
  readonly ForceHTTP: boolean
}

export type WorkspaceBuildTransition = "start" | "stop" | "delete"

export type Message = { message: string }
