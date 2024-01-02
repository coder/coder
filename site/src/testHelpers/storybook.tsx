import { DashboardProviderContext } from "components/Dashboard/DashboardProvider";
import {
  MockAppearanceConfig,
  MockBuildInfo,
  MockEntitlements,
} from "./entities";
import { FC } from "react";
import { StoryContext } from "@storybook/react";
import * as _storybook_types from "@storybook/react";
import { Entitlements } from "api/typesGenerated";
import { withDefaultFeatures } from "api/api";

export const withDashboardProvider = (
  Story: FC,
  { parameters }: StoryContext,
) => {
  const { features = [], experiments = [] } = parameters;

  const entitlements: Entitlements = {
    ...MockEntitlements,
    features: withDefaultFeatures(
      features.reduce(
        (acc, feature) => {
          acc[feature] = { enabled: true, entitlement: "entitled" };
          return acc;
        },
        {} as Entitlements["features"],
      ),
    ),
  };

  return (
    <DashboardProviderContext.Provider
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
    </DashboardProviderContext.Provider>
  );
};
