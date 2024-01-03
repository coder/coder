import type { Meta, StoryObj } from "@storybook/react";
import { ScheduleForm } from "./ScheduleForm";
import { mockApiError } from "testHelpers/entities";
import { action } from "@storybook/addon-actions";

const defaultArgs = {
  submitting: false,
  initialValues: {
    raw_schedule: "CRON_TZ=Australia/Sydney 0 2 * * *",
    user_set: false,
    user_can_set: true,
    time: "02:00",
    timezone: "Australia/Sydney",
    next: "2023-09-05T02:00:00+10:00",
  },
  updateErr: undefined,
  now: new Date("2023-09-04T15:00:00+10:00"),
  onSubmit: action("onSubmit"),
};

const meta: Meta<typeof ScheduleForm> = {
  title: "pages/UserSettingsPage/ScheduleForm",
  component: ScheduleForm,
  args: defaultArgs,
};
export default meta;

type Story = StoryObj<typeof ScheduleForm>;

export const ExampleDefault: Story = {};

export const ExampleUserSet: Story = {
  args: {
    initialValues: {
      raw_schedule: "CRON_TZ=America/Chicago 0 2 * * *",
      user_set: true,
      user_can_set: true,
      time: "02:00",
      timezone: "America/Chicago",
      next: "2023-09-05T02:00:00-05:00",
    },
    now: new Date("2023-09-04T15:00:00-05:00"),
  },
};

export const Submitting: Story = {
  args: {
    isLoading: true,
  },
};

export const WithError: Story = {
  args: {
    submitError: mockApiError({
      message: "Invalid schedule",
      validations: [
        {
          field: "schedule",
          detail: "Could not validate cron schedule.",
        },
      ],
    }),
  },
};
