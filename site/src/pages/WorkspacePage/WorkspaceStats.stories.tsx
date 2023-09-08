import { Story } from "@storybook/react";
import {
  MockWorkspace,
  MockAppearance,
  MockBuildInfo,
  MockEntitlementsWithScheduling,
  MockExperiments,
} from "testHelpers/entities";
import { WorkspaceStats, WorkspaceStatsProps } from "./WorkspaceStats";
import { DashboardProviderContext } from "components/Dashboard/DashboardProvider";

export default {
  title: "components/WorkspaceStats",
  component: WorkspaceStats,
};

const MockedAppearance = {
  config: MockAppearance,
  preview: false,
  setPreview: () => null,
  save: () => null,
};

const Template: Story<WorkspaceStatsProps> = (args) => (
  <DashboardProviderContext.Provider
    value={{
      buildInfo: MockBuildInfo,
      entitlements: MockEntitlementsWithScheduling,
      experiments: MockExperiments,
      appearance: MockedAppearance,
    }}
  >
    <WorkspaceStats {...args} />
  </DashboardProviderContext.Provider>
);

export const Example = Template.bind({});
Example.args = {
  workspace: MockWorkspace,
};

export const Outdated = Template.bind({});
Outdated.args = {
  workspace: {
    ...MockWorkspace,
    outdated: true,
  },
};
