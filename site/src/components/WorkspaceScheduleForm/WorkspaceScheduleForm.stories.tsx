import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import * as Mocks from "../../testHelpers/entities"
import { defaultWorkspaceSchedule, WorkspaceScheduleForm, WorkspaceScheduleFormProps } from "./WorkspaceScheduleForm"

dayjs.extend(advancedFormat)
dayjs.extend(utc)
dayjs.extend(timezone)

export default {
  title: "components/WorkspaceScheduleForm",
  component: WorkspaceScheduleForm,
}

const Template: Story<WorkspaceScheduleFormProps> = (args) => <WorkspaceScheduleForm {...args} />

export const WorkspaceNotRunning = Template.bind({})
WorkspaceNotRunning.args = {
  now: dayjs("2022-05-17T17:40:00Z"),
  initialValues: {
    ...defaultWorkspaceSchedule(5),
    timezone: "UTC",
  },
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
      updated_at: "2022-05-17T17:39:00Z",
    },
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

export const WorkspaceWillNotShutDown = Template.bind({})
WorkspaceWillNotShutDown.args = {
  now: dayjs("2022-05-17T17:40:00Z"),
  initialValues: {
    ...defaultWorkspaceSchedule(5),
    timezone: "UTC",
    ttl: 0,
  },
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      updated_at: "2022-05-17T17:39:00Z",
    },
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

export const WorkspaceWillShutdown = Template.bind({})
WorkspaceWillShutdown.args = {
  now: dayjs("2022-05-17T17:40:00Z"),
  initialValues: {
    ...defaultWorkspaceSchedule(5),
    timezone: "UTC",
  },
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      updated_at: "2022-05-17T17:39:00Z",
    },
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

export const WorkspaceWillShutdownSoon = Template.bind({})
WorkspaceWillShutdownSoon.args = {
  now: dayjs("2022-05-17T16:39:00Z"),
  initialValues: {
    ...defaultWorkspaceSchedule(2),
    timezone: "UTC",
    ttl: 1,
  },
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: "2022-05-17T18:09:00Z",
    },
    ttl_ms: 2 * 60 * 60 * 1000, // 2 hours = shuts off at 18:09
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

export const WorkspaceWillShutdownImmediately = Template.bind({})
WorkspaceWillShutdownImmediately.args = {
  now: dayjs("2022-05-17T17:09:00Z"),
  initialValues: {
    ...defaultWorkspaceSchedule(1),
    timezone: "UTC",
    ttl: 1,
  },
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: "2022-05-17T18:09:00Z",
    },
    ttl_ms: 2 * 60 * 60 * 1000, // 2 hours = shuts off at 18:09
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}
