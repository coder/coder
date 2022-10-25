import { Story } from "@storybook/react"
import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import * as Mocks from "../../testHelpers/entities"
import {
  WorkspaceScheduleButton,
  WorkspaceScheduleButtonProps,
} from "./WorkspaceScheduleButton"

dayjs.extend(utc)

export default {
  title: "components/WorkspaceScheduleButton",
  component: WorkspaceScheduleButton,
  argTypes: {
    canUpdateWorkspace: {
      defaultValue: true,
    },
    deadlineMinusEnabled: {
      defaultValue: (): boolean => false,
    },
    deadlinePlusEnabled: {
      defaultValue: (): boolean => false,
    },
  },
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
  deadlinePlusEnabled: () => true,
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
    },
    ttl_ms: 2 * 365 * 24 * 60 * 60 * 1000, // 2 years
  },
}

export const WorkspaceOn = Template.bind({})
WorkspaceOn.args = {
  deadlineMinusEnabled: () => true,
  deadlinePlusEnabled: () => true,
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "start",
      deadline: "2022-05-17T23:59:00.00Z",
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

export const SmallViewport = Template.bind({})
SmallViewport.args = {
  ...WorkspaceOffShort.args,
}
SmallViewport.parameters = {
  chromatic: { viewports: [320] },
}
