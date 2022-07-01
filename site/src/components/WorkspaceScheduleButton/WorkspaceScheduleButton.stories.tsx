import { Story } from "@storybook/react"
import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import * as Mocks from "../../testHelpers/entities"
import { WorkspaceScheduleButton, WorkspaceScheduleButtonProps } from "./WorkspaceScheduleButton"

dayjs.extend(utc)

// REMARK: There's a known problem with storybook and using date libraries that
//         call string.toLowerCase
// SEE: https:github.com/storybookjs/storybook/issues/12208#issuecomment-697044557
const ONE = 1
const SEVEN = 7
const THIRTY = 30

export default {
  title: "components/WorkspaceScheduleButton",
  component: WorkspaceScheduleButton,
}

const Template: Story<WorkspaceScheduleButtonProps> = (args) => (
  <WorkspaceScheduleButton {...args} />
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
      // a mannual shutdown has a deadline of '"0001-01-01T00:00:00Z"'
      // SEE: #1834
      deadline: "0001-01-01T00:00:00Z",
    },
    ttl_ms: undefined,
  },
}

export const ShutdownRealSoon = Template.bind({})
ShutdownRealSoon.args = {
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: dayjs().add(THIRTY, "minute").utc().format(),
      transition: "start",
    },
    ttl_ms: 2 * 60 * 60 * 1000, // 2 hours
  },
}

export const ShutdownSoon = Template.bind({})
ShutdownSoon.args = {
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: dayjs().add(ONE, "hour").utc().format(),
      transition: "start",
    },
    ttl_ms: 2 * 60 * 60 * 1000, // 2 hours
  },
}

export const ShutdownLong = Template.bind({})
ShutdownLong.args = {
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: dayjs().add(SEVEN, "days").utc().format(),
      transition: "start",
    },
    ttl_ms: 7 * 24 * 60 * 60 * 1000, // 7 days
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
