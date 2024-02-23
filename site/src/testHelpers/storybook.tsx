import { type FC } from "react";
import type { StoryContext } from "@storybook/react";
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

export const withWSMessages = (Story: FC, { parameters }: StoryContext) => {
  if (!parameters.messages) {
    console.warn("Looks like you forgot to add messages to the story");
  }

  // @ts-expect-error -- TS doesn't know about the global WebSocket
  window.WebSocket = function () {
    return {
      addEventListener: (
        type: string,
        callback: (ev: Record<"data", string>) => void,
      ) => {
        if (type === "message") {
          parameters.messages?.forEach((message) => {
            callback({ data: message });
          });
        }
      },
      close: () => {},
    };
  };

  return <Story />;
};
