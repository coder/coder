import { Meta, StoryObj } from "@storybook/react";
import dayjs from "dayjs";
import advancedFormat from "dayjs/plugin/advancedFormat";
import timezone from "dayjs/plugin/timezone";
import utc from "dayjs/plugin/utc";
import {
  defaultSchedule,
  emptySchedule,
} from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/schedule";
import { emptyTTL } from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/ttl";
import { mockApiError } from "testHelpers/entities";
import { WorkspaceScheduleForm } from "./WorkspaceScheduleForm";

dayjs.extend(advancedFormat);
dayjs.extend(utc);
dayjs.extend(timezone);

const meta: Meta<typeof WorkspaceScheduleForm> = {
  title: "components/WorkspaceScheduleForm",
  component: WorkspaceScheduleForm,
};

export default meta;
type Story = StoryObj<typeof WorkspaceScheduleForm>;

const defaultInitialValues = {
  autostartEnabled: true,
  ...defaultSchedule(),
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
    submitScheduleError: mockApiError({
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
