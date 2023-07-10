import { Story } from "@storybook/react"
import {
  MockCanceledWorkspace,
  MockCancelingWorkspace,
  MockDeletedWorkspace,
  MockDeletingWorkspace,
  MockFailedWorkspace,
  MockPendingWorkspace,
  MockStartingWorkspace,
  MockStoppedWorkspace,
  MockStoppingWorkspace,
  MockWorkspace,
  MockBuildInfo,
  MockEntitlementsWithScheduling,
  MockExperiments,
  MockAppearance,
} from "testHelpers/entities"
import {
  WorkspaceHealthBadge,
  WorkspaceHealthBadgeProps,
} from "./WorkspaceHealthBadge"
import { DashboardProviderContext } from "components/Dashboard/DashboardProvider"

export default {
  title: "components/WorkspaceHealthBadge",
  component: WorkspaceHealthBadge,
}

const MockedAppearance = {
  config: MockAppearance,
  preview: false,
  setPreview: () => null,
  save: () => null,
}

const Template: Story<WorkspaceHealthBadgeProps> = (args) => (
  <DashboardProviderContext.Provider
    value={{
      buildInfo: MockBuildInfo,
      entitlements: MockEntitlementsWithScheduling,
      experiments: MockExperiments,
      appearance: MockedAppearance,
    }}
  >
    <WorkspaceHealthBadge {...args} />
  </DashboardProviderContext.Provider>
)

export const Running = Template.bind({})
Running.args = {
  workspace: MockWorkspace,
}

// TODO(mafredri): Healthyness mocks.

export const Starting = Template.bind({})
Starting.args = {
  workspace: MockStartingWorkspace,
}

export const Stopped = Template.bind({})
Stopped.args = {
  workspace: MockStoppedWorkspace,
}

export const Stopping = Template.bind({})
Stopping.args = {
  workspace: MockStoppingWorkspace,
}

export const Deleting = Template.bind({})
Deleting.args = {
  workspace: MockDeletingWorkspace,
}

export const Deleted = Template.bind({})
Deleted.args = {
  workspace: MockDeletedWorkspace,
}

export const Canceling = Template.bind({})
Canceling.args = {
  workspace: MockCancelingWorkspace,
}

export const Canceled = Template.bind({})
Canceled.args = {
  workspace: MockCanceledWorkspace,
}

export const Failed = Template.bind({})
Failed.args = {
  workspace: MockFailedWorkspace,
}

export const Pending = Template.bind({})
Pending.args = {
  workspace: MockPendingWorkspace,
}
