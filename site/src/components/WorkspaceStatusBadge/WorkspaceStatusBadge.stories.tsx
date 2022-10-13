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
} from "testHelpers/renderHelpers"
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
  build: MockWorkspace.latest_build,
}

export const Starting = Template.bind({})
Starting.args = {
  build: MockStartingWorkspace.latest_build,
}

export const Stopped = Template.bind({})
Stopped.args = {
  build: MockStoppedWorkspace.latest_build,
}

export const Stopping = Template.bind({})
Stopping.args = {
  build: MockStoppingWorkspace.latest_build,
}

export const Deleting = Template.bind({})
Deleting.args = {
  build: MockDeletingWorkspace.latest_build,
}

export const Deleted = Template.bind({})
Deleted.args = {
  build: MockDeletedWorkspace.latest_build,
}

export const Canceling = Template.bind({})
Canceling.args = {
  build: MockCancelingWorkspace.latest_build,
}

export const Canceled = Template.bind({})
Canceled.args = {
  build: MockCanceledWorkspace.latest_build,
}

export const Failed = Template.bind({})
Failed.args = {
  build: MockFailedWorkspace.latest_build,
}

export const Pending = Template.bind({})
Pending.args = {
  build: MockPendingWorkspace.latest_build,
}
