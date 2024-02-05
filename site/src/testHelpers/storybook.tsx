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
