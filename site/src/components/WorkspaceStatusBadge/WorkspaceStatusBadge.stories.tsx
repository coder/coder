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
} from "testHelpers/entities"
import {
  WorkspaceStatusBadge,
  WorkspaceStatusBadgeProps,
} from "./WorkspaceStatusBadge"

export default {
  title: "components/WorkspaceStatusBadge",
  component: WorkspaceStatusBadge,
}

const Template: Story<WorkspaceStatusBadgeProps> = (args) => (
  <WorkspaceStatusBadge {...args} />
)

export const Running = Template.bind({})
Running.args = {
  workspace: MockWorkspace,
}

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
