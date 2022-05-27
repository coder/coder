import { Story } from "@storybook/react"
import dayjs from "dayjs"
import React from "react"
import * as Mocks from "../../testHelpers/entities"
import { WorkspaceSchedule, WorkspaceScheduleProps } from "./WorkspaceSchedule"

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
    ttl: undefined,
  },
}

export const ShutdownSoon = Template.bind({})
ShutdownSoon.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "start",
      updated_at: dayjs().subtract(1, "hour").toString(), // 1 hour ago
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
      transition: "start",
      updated_at: dayjs().toString(),
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
