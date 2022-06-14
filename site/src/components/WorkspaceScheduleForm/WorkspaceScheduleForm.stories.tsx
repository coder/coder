import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { defaultWorkspaceSchedule, WorkspaceScheduleForm, WorkspaceScheduleFormProps } from "./WorkspaceScheduleForm"

dayjs.extend(advancedFormat)
dayjs.extend(utc)
dayjs.extend(timezone)

export default {
  title: "components/WorkspaceScheduleForm",
  component: WorkspaceScheduleForm,
}

const Template: Story<WorkspaceScheduleFormProps> = (args) => <WorkspaceScheduleForm {...args} />

export const WorkspaceWillNotShutDown = Template.bind({})
WorkspaceWillNotShutDown.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(5),
    ttl: 0,
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

export const WorkspaceWillShutdownInAnHour = Template.bind({})
WorkspaceWillShutdownInAnHour.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(5),
    ttl: 1,
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

export const WorkspaceWillShutdownInTwoHours = Template.bind({})
WorkspaceWillShutdownInTwoHours.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(2),
    ttl: 2,
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

export const WorkspaceWillShutdownInADay = Template.bind({})
WorkspaceWillShutdownInADay.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(2),
    ttl: 24,
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

export const WorkspaceWillShutdownInTwoDays = Template.bind({})
WorkspaceWillShutdownInTwoDays.args = {
  initialValues: {
    ...defaultWorkspaceSchedule(2),
    ttl: 48,
  },
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}
