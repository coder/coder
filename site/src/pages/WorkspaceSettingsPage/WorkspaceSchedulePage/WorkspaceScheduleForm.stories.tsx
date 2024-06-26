import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import dayjs from "dayjs";
import advancedFormat from "dayjs/plugin/advancedFormat";
import timezone from "dayjs/plugin/timezone";
import utc from "dayjs/plugin/utc";
import {
  defaultSchedule,
  emptySchedule,
} from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/schedule";
import { emptyTTL } from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/ttl";
import { MockTemplate, mockApiError } from "testHelpers/entities";
import { WorkspaceScheduleForm } from "./WorkspaceScheduleForm";

dayjs.extend(advancedFormat);
dayjs.extend(utc);
dayjs.extend(timezone);

const mockTemplate = {
  ...MockTemplate,
  allow_user_autostart: true,
  allow_user_autostop: true,
  autostart_requirement: {
    days_of_week: [
      "sunday",
      "monday",
      "tuesday",
      "wednesday",
      "thursday",
      "friday",
      "saturday",
    ],
  },
};

const meta: Meta<typeof WorkspaceScheduleForm> = {
  title: "pages/WorkspaceSettingsPage/WorkspaceScheduleForm",
  component: WorkspaceScheduleForm,
  args: {
    template: mockTemplate,
    onCancel: action("onCancel"),
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceScheduleForm>;

const defaultInitialValues = {
  ...defaultSchedule(),
  autostartEnabled: true,
  autostopEnabled: true,
  ttl: 24,
};

export const AllDisabled: Story = {
  args: {
    initialValues: {
      autostartEnabled: false,
      ...emptySchedule,
      autostopEnabled: false,
      ttl: emptyTTL,
    },
    template: {
      ...mockTemplate,
      allow_user_autostart: false,
      allow_user_autostop: false,
    },
  },
};

export const Autostart: Story = {
  args: {
    initialValues: {
      autostartEnabled: true,
      ...defaultSchedule(),
      autostopEnabled: false,
      ttl: emptyTTL,
    },
    template: {
      ...mockTemplate,
      allow_user_autostop: false,
    },
  },
};

export const WorkspaceWillShutdownInTwoHours: Story = {
  args: {
    initialValues: { ...defaultInitialValues, ttl: 2 },
  },
};

export const WorkspaceWillShutdownInADay: Story = {
  args: {
    initialValues: { ...defaultInitialValues, ttl: 24 },
  },
};

export const WorkspaceWillShutdownInTwoDays: Story = {
  args: {
    initialValues: { ...defaultInitialValues, ttl: 48 },
  },
};

export const WithError: Story = {
  args: {
    initialValues: { ...defaultInitialValues, ttl: 100 },
    initialTouched: { ttl: true },
    error: mockApiError({
      message: "Something went wrong.",
      validations: [
        { field: "ttl_ms", detail: "Invalid time until shutdown." },
      ],
    }),
  },
};

export const Loading: Story = {
  args: {
    initialValues: defaultInitialValues,
    isLoading: true,
  },
};

export const AutoStopAndStartOff: Story = {
  args: {
    initialValues: {
      ...defaultInitialValues,
      autostartEnabled: false,
      autostopEnabled: false,
    },
  },
};
