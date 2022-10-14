import { Story } from "@storybook/react"
import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import * as Mocks from "../../testHelpers/entities"
import { WorkspaceSchedule, WorkspaceScheduleProps } from "./WorkspaceSchedule"

dayjs.extend(utc)

export default {
  title: "components/WorkspaceSchedule",
  component: WorkspaceSchedule,
  argTypes: {
    canUpdateWorkspace: {
      defaultValue: true,
    },
  },
}

const Template: Story<WorkspaceScheduleProps> = (args) => (
  <WorkspaceSchedule {...args} />
)

export const NoScheduleNoTTL = Template.bind({})
NoScheduleNoTTL.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
    },
    autostart_schedule: undefined,
    ttl_ms: undefined,
  },
}

export const NoTTL = Template.bind({})
NoTTL.args = {
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: undefined,
    },
    ttl_ms: undefined,
  },
}

export const WorkspaceOffShort = Template.bind({})
WorkspaceOffShort.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
    },
    ttl_ms: 2 * 60 * 60 * 1000, // 2 hours
  },
}

export const WorkspaceOffLong = Template.bind({})
WorkspaceOffLong.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
    },
    ttl_ms: 2 * 365 * 24 * 60 * 60 * 1000, // 2 years
  },
}

export const CannotEdit = Template.bind({})
CannotEdit.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
    },
    ttl_ms: 2 * 60 * 60 * 1000, // 2 hours
  },
  canUpdateWorkspace: false,
}
