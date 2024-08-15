import * as _storybook_types from "@storybook/react";
import type {
  DeploymentValues,
  Experiments,
  FeatureName,
  SerpentOption,
  User,
} from "api/typesGenerated";
import type { Permissions } from "contexts/auth/permissions";
import type { QueryKey } from "react-query";

declare module "@storybook/react" {
  type WebSocketEvent =
    | { event: "message"; data: string }
    | { event: "error" | "close" };
  interface Parameters {
    features?: FeatureName[];
    experiments?: Experiments;
    queries?: { key: QueryKey; data: unknown }[];
    webSocket?: WebSocketEvent[];
    user?: User;
    permissions?: Partial<Permissions>;
    deploymentValues?: DeploymentValues;
    deploymentOptions?: SerpentOption[];
  }
}
