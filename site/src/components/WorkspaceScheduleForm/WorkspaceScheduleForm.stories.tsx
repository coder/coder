import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import * as Mocks from "../../testHelpers/entities"
import {
  WorkspaceScheduleForm,
  defaultWorkspaceSchedule,
  WorkspaceScheduleFormProps,
} from "./WorkspaceScheduleForm"

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
    ...defaultWorkspaceSchedule(5, "asdfasdf"),
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
    ...defaultWorkspaceSchedule(5, "asdfasdf"),
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
    ...defaultWorkspaceSchedule(5, "asdfasdf"),
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
  now: dayjs("2022-05-17T18:10:00Z"),
  initialValues: {
    ...defaultWorkspaceSchedule(5, "asdfasdf"),
    timezone: "UTC",
    ttl: 1,
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

export const WorkspaceWillShutdownImmediately = Template.bind({})
WorkspaceWillShutdownImmediately.args = {
  now: dayjs("2022-05-17T18:40:00Z"),
  initialValues: {
    ...defaultWorkspaceSchedule(5, "asdfasdf"),
    timezone: "UTC",
    ttl: 1,
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
