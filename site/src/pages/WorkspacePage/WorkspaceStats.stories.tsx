import { Meta, StoryObj } from "@storybook/react";
import {
  MockWorkspace,
  MockAppearanceConfig,
  MockBuildInfo,
  MockEntitlementsWithScheduling,
  MockExperiments,
} from "testHelpers/entities";
import { WorkspaceStats } from "./WorkspaceStats";
import { DashboardProviderContext } from "components/Dashboard/DashboardProvider";

const MockedAppearance = {
  config: MockAppearanceConfig,
  isPreview: false,
  setPreview: () => {},
};

const meta: Meta<typeof WorkspaceStats> = {
  title: "components/WorkspaceStats",
  component: WorkspaceStats,
  decorators: [
    (Story) => (
      <DashboardProviderContext.Provider
        value={{
          buildInfo: MockBuildInfo,
          entitlements: MockEntitlementsWithScheduling,
          experiments: MockExperiments,
          appearance: MockedAppearance,
        }}
      >
        <Story />
      </DashboardProviderContext.Provider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspaceStats>;

export const Example: Story = {
  args: {
    workspace: MockWorkspace,
  },
};

export const Outdated: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      outdated: true,
    },
  },
};
