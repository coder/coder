import { Story } from "@storybook/react"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { makeMockApiError } from "testHelpers/entities"
import {
  defaultWorkspaceSchedule,
  WorkspaceScheduleForm,
  WorkspaceScheduleFormProps,
} from "./WorkspaceScheduleForm"

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
  },
}

const Template: Story<WorkspaceScheduleFormProps> = (args) => <WorkspaceScheduleForm {...args} />

export const WorkspaceWillNotShutDown = Template.bind({})
WorkspaceWillNotShutDown.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(5),
    ttl: 0,
  },
}

export const WorkspaceWillShutdownInAnHour = Template.bind({})
WorkspaceWillShutdownInAnHour.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(5),
    ttl: 1,
  },
}

export const WorkspaceWillShutdownInTwoHours = Template.bind({})
WorkspaceWillShutdownInTwoHours.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(2),
    ttl: 2,
  },
}

export const WorkspaceWillShutdownInADay = Template.bind({})
WorkspaceWillShutdownInADay.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(2),
    ttl: 24,
  },
}

export const WorkspaceWillShutdownInTwoDays = Template.bind({})
WorkspaceWillShutdownInTwoDays.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(2),
    ttl: 48,
  },
}

export const WithError = Template.bind({})
WithError.args = {
  initialTouched: { ttl: true },
  submitScheduleError: makeMockApiError({
    message: "Something went wrong.",
    validations: [{ field: "ttl_ms", detail: "Invalid time until shutdown." }],
  }),
}

export const Loading = Template.bind({})
Loading.args = { isLoading: true }
