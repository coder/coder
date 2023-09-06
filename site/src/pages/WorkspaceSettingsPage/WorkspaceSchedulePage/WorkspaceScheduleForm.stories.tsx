import { Story } from "@storybook/react";
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
import {
  WorkspaceScheduleForm,
  WorkspaceScheduleFormProps,
} from "./WorkspaceScheduleForm";

dayjs.extend(advancedFormat);
dayjs.extend(utc);
dayjs.extend(timezone);

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
};

const Template: Story<WorkspaceScheduleFormProps> = (args) => (
  <WorkspaceScheduleForm {...args} />
);

const defaultInitialValues = {
  autostartEnabled: true,
  ...defaultSchedule(),
  autostopEnabled: true,
  ttl: 24,
};

export const AllDisabled = Template.bind({});
AllDisabled.args = {
  initialValues: {
    autostartEnabled: false,
    ...emptySchedule,
    autostopEnabled: false,
    ttl: emptyTTL,
  },
};

export const Autostart = Template.bind({});
Autostart.args = {
  initialValues: {
    autostartEnabled: true,
    ...defaultSchedule(),
    autostopEnabled: false,
    ttl: emptyTTL,
  },
};

export const WorkspaceWillShutdownInTwoHours = Template.bind({});
WorkspaceWillShutdownInTwoHours.args = {
  initialValues: { ...defaultInitialValues, ttl: 2 },
};

export const WorkspaceWillShutdownInADay = Template.bind({});
WorkspaceWillShutdownInADay.args = {
  initialValues: { ...defaultInitialValues, ttl: 24 },
};

export const WorkspaceWillShutdownInTwoDays = Template.bind({});
WorkspaceWillShutdownInTwoDays.args = {
  initialValues: { ...defaultInitialValues, ttl: 48 },
};

export const WithError = Template.bind({});
WithError.args = {
  initialValues: { ...defaultInitialValues, ttl: 100 },
  initialTouched: { ttl: true },
  submitScheduleError: mockApiError({
    message: "Something went wrong.",
    validations: [{ field: "ttl_ms", detail: "Invalid time until shutdown." }],
  }),
};

export const Loading = Template.bind({});
Loading.args = {
  initialValues: defaultInitialValues,
  isLoading: true,
};
