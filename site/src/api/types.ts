import { DeploymentValues } from "./typesGenerated";

export interface UserAgent {
  readonly browser: string;
  readonly device: string;
  readonly ip_address: string;
  readonly os: string;
}

export interface ReconnectingPTYRequest {
  readonly data?: string;
  readonly height?: number;
  readonly width?: number;
}

export type WorkspaceBuildTransition = "start" | "stop" | "delete";

export type Message = { message: string };

export interface DeploymentGroup {
  readonly name: string;
  readonly parent?: DeploymentGroup;
  readonly description: string;
  readonly children: DeploymentGroup[];
}

export interface DeploymentOption {
  readonly name: string;
  readonly description: string;
  readonly flag: string;
  readonly flag_shorthand: string;
  readonly value: unknown;
  readonly hidden: boolean;
  readonly group?: DeploymentGroup;
}

export type DeploymentConfig = {
  readonly config: DeploymentValues;
  readonly options: DeploymentOption[];
};
