import { Story } from "@storybook/react"
import { ScheduleForm, ScheduleFormProps } from "./ScheduleForm"
import { mockApiError } from "testHelpers/entities"

export default {
  title: "pages/UserSettingsPage/SchedulePage/ScheduleForm",
  component: ScheduleForm,
  argTypes: {
    onSubmit: { action: "Submit" },
  },
}

const Template: Story<ScheduleFormProps> = (args: ScheduleFormProps) => (
  <ScheduleForm {...args} />
)

export const ExampleDefault = Template.bind({})
ExampleDefault.args = {
  submitting: false,
  initialValues: {
    raw_schedule: "CRON_TZ=Australia/Sydney 0 2 * * *",
    user_set: false,
    time: "02:00",
    timezone: "Australia/Sydney",
    next: "2023-09-05T02:00:00+10:00",
  },
  updateErr: undefined,
  onSubmit: () => {
    return Promise.resolve()
  },
  now: new Date("2023-09-04T15:00:00+10:00"),
}

export const ExampleUserSet = Template.bind({})
ExampleUserSet.args = {
  ...ExampleDefault.args,
  initialValues: {
    raw_schedule: "CRON_TZ=America/Chicago 0 2 * * *",
    user_set: true,
    time: "02:00",
    timezone: "America/Chicago",
    next: "2023-09-05T02:00:00-05:00",
  },
  now: new Date("2023-09-04T15:00:00-05:00"),
}

export const Submitting = Template.bind({})
Submitting.args = {
  ...ExampleDefault.args,
  submitting: true,
}

export const WithError = Template.bind({})
WithError.args = {
  ...ExampleDefault.args,
  updateErr: mockApiError({
    message: "Invalid schedule",
    validations: [
      {
        field: "schedule",
        detail: "Could not validate cron schedule.",
      },
    ],
  }),
}
