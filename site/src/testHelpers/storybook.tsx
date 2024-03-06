import type { StoryContext } from "@storybook/react";
import type { FC } from "react";
import { withDefaultFeatures } from "api/api";
import type { Entitlements } from "api/typesGenerated";
import { DashboardContext } from "modules/dashboard/DashboardProvider";
import {
  MockAppearanceConfig,
  MockBuildInfo,
  MockEntitlements,
} from "./entities";

export const withDashboardProvider = (
  Story: FC,
  { parameters }: StoryContext,
) => {
  const { features = [], experiments = [] } = parameters;

  const entitlements: Entitlements = {
    ...MockEntitlements,
    features: withDefaultFeatures(
      Object.fromEntries(
        features.map((feature) => [
          feature,
          { enabled: true, entitlement: "entitled" },
        ]),
      ),
    ),
  };

  return (
    <DashboardContext.Provider
      value={{
        buildInfo: MockBuildInfo,
        entitlements,
        experiments,
        appearance: {
          config: MockAppearanceConfig,
          isPreview: false,
          setPreview: () => {},
        },
      }}
    >
      <Story />
    </DashboardContext.Provider>
  );
};

export const withWebSocket = (Story: FC, { parameters }: StoryContext) => {
  const webSocketConfig = parameters.webSocket;

  if (!webSocketConfig) {
    console.warn("Your forgot to add the `parameters.webSocket` to your story");
    return <Story />;
  }

  // @ts-expect-error -- TS doesn't know about the global WebSocket
  window.WebSocket = function () {
    return {
      addEventListener: (
        type: string,
        callback: (ev?: Record<"data", string>) => void,
      ) => {
        if (type === "message" && webSocketConfig.event === "message") {
          webSocketConfig.messages.forEach((message) => {
            callback({ data: message });
          });
        }
        if (type === "error" && webSocketConfig.event === "error") {
          callback();
        }
      },
      close: () => {},
    };
  };

  return <Story />;
};
