import { Story } from "@storybook/react"
import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import * as Mocks from "../../testHelpers/entities"
import { WorkspaceSchedule, WorkspaceScheduleProps } from "./WorkspaceSchedule"

dayjs.extend(utc)

export default {
  title: "components/WorkspaceSchedule",
  component: WorkspaceSchedule,
  argTypes: {},
}

const Template: Story<WorkspaceScheduleProps> = (args) => <WorkspaceSchedule {...args} />

export const NoTTL = Template.bind({})
NoTTL.args = {
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      // a mannual shutdown has a deadline of '"0001-01-01T00:00:00Z"'
      // SEE: #1834
      deadline: "0001-01-01T00:00:00Z",
    },
    ttl: undefined,
  },
}

export const ShutdownSoon = Template.bind({})
ShutdownSoon.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: dayjs().utc().add(1, "hour").toString(), // in 1 hour ago
      job: {
        ...Mocks.MockProvisionerJob,
      },
      transition: "start",
    },
    ttl: 2 * 60 * 60 * 1000 * 1_000_000, // 2 hours
  },
}

export const ShutdownLong = Template.bind({})
ShutdownLong.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: dayjs().utc().add(7, "days").toString(), // in 7 days
      transition: "start",
    },
    ttl: 7 * 24 * 60 * 60 * 1000 * 1_000_000, // 7 days
  },
}

export const WorkspaceOffShort = Template.bind({})
WorkspaceOffShort.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
      updated_at: dayjs().subtract(2, "days").toString(),
    },
    ttl: 2 * 60 * 60 * 1000 * 1_000_000, // 2 hours
  },
}

export const WorkspaceOffLong = Template.bind({})
WorkspaceOffLong.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
      updated_at: dayjs().subtract(2, "days").toString(),
    },
    ttl: 2 * 365 * 24 * 60 * 60 * 1000 * 1_000_000, // 2 years
  },
}
