import { Story } from "@storybook/react"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { defaultSchedule, emptySchedule } from "pages/WorkspacesPage/schedule"
import { emptyTTL } from "pages/WorkspacesPage/ttl"
import { makeMockApiError } from "testHelpers/entities"
import { WorkspaceScheduleForm, WorkspaceScheduleFormProps } from "./WorkspaceScheduleForm"

dayjs.extend(advancedFormat)
dayjs.extend(utc)
dayjs.extend(timezone)

export default {
  title: "components/WorkspaceScheduleForm",
  component: WorkspaceScheduleForm,
  argTypes: {
    onCancel: {
      action: "onCancel",
    },
    onSubmit: {
      action: "onSubmit",
    },
    toggleAutoStart: {
      action: "toggleAutoStart",
    },
    toggleAutoStop: {
      action: "toggleAutoStop",
    },
  },
}

const Template: Story<WorkspaceScheduleFormProps> = (args) => <WorkspaceScheduleForm {...args} />

export const AllDisabled = Template.bind({})
AllDisabled.args = {
  autoStart: { enabled: false, schedule: emptySchedule },
  autoStop: { enabled: false, ttl: emptyTTL },
}

export const AutoStart = Template.bind({})
AutoStart.args = {
  autoStart: { enabled: true, schedule: defaultSchedule() },
  autoStop: { enabled: false, ttl: emptyTTL },
}

export const WorkspaceWillShutdownInTwoHours = Template.bind({})
WorkspaceWillShutdownInTwoHours.args = {
  autoStart: { enabled: true, schedule: defaultSchedule() },
  autoStop: { enabled: true, ttl: 2 },
}

export const WorkspaceWillShutdownInADay = Template.bind({})
WorkspaceWillShutdownInADay.args = {
  autoStart: { enabled: true, schedule: defaultSchedule() },
  autoStop: { enabled: true, ttl: 24 },
}

export const WorkspaceWillShutdownInTwoDays = Template.bind({})
WorkspaceWillShutdownInTwoDays.args = {
  autoStart: { enabled: true, schedule: defaultSchedule() },
  autoStop: { enabled: true, ttl: 48 },
}

export const WithError = Template.bind({})
WithError.args = {
  autoStart: { enabled: false, schedule: emptySchedule },
  autoStop: { enabled: true, ttl: 100 },
  initialTouched: { ttl: true },
  submitScheduleError: makeMockApiError({
    message: "Something went wrong.",
    validations: [{ field: "ttl_ms", detail: "Invalid time until shutdown." }],
  }),
}

export const Loading = Template.bind({})
Loading.args = {
  autoStart: { enabled: true, schedule: defaultSchedule() },
  autoStop: { enabled: true, ttl: 2 },
  isLoading: true,
}
