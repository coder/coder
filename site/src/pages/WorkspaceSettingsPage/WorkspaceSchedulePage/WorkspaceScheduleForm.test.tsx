import {
  Language,
  ttlShutdownAt,
  validationSchema,
  WorkspaceScheduleFormValues,
  WorkspaceScheduleForm,
} from "./WorkspaceScheduleForm";
import { timeZones } from "utils/timeZones";
import * as API from "api/api";
import { MockTemplate } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { defaultSchedule } from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/schedule";
import { screen } from "@testing-library/react";

const valid: WorkspaceScheduleFormValues = {
  autostartEnabled: true,
  sunday: false,
  monday: true,
  tuesday: true,
  wednesday: true,
  thursday: true,
  friday: true,
  saturday: false,
  startTime: "09:30",
  timezone: "Canada/Eastern",

  autostopEnabled: true,
  ttl: 120,
};

describe("validationSchema", () => {
  it("allows everything to be falsy when switches are off", () => {
    const values: WorkspaceScheduleFormValues = {
      autostartEnabled: false,
      sunday: false,
      monday: false,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,
      startTime: "",
      timezone: "",

      autostopEnabled: false,
      ttl: 0,
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).not.toThrow();
  });

  it("disallows ttl to be negative", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: -1,
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrow();
  });

  it("disallows all days-of-week to be false when autostart is enabled", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      sunday: false,
      monday: false,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrowError(Language.errorNoDayOfWeek);
  });

  it("disallows empty startTime when autostart is enabled", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      sunday: false,
      monday: true,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,
      startTime: "",
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrowError(Language.errorNoTime);
  });

  it("allows startTime 16:20", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "16:20",
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).not.toThrow();
  });

  it("disallows startTime to be H:mm", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "9:30",
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrowError(Language.errorTime);
  });

  it("disallows startTime to be HH:m", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "09:5",
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrowError(Language.errorTime);
  });

  it("disallows an invalid startTime 24:01", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "24:01",
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrowError(Language.errorTime);
  });

  it("disallows an invalid startTime 09:60", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "09:60",
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrowError(Language.errorTime);
  });

  it("disallows an invalid timezone Canada/North", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      timezone: "Canada/North",
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrowError(Language.errorTimezone);
  });

  it.each<[string]>(timeZones.map((zone) => [zone]))(
    `validation passes for tz=%p`,
    (zone) => {
      const values: WorkspaceScheduleFormValues = {
        ...valid,
        timezone: zone,
      };
      const validate = () => validationSchema.validateSync(values);
      expect(validate).not.toThrow();
    },
  );

  it("allows a ttl of 7 days", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: 24 * 7,
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("allows a ttl of 30 days", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: 24 * 30,
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("disallows a ttl of 30 days + 1 hour", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: 24 * 30 + 1,
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).toThrowError(Language.errorTtlMax);
  });

  it("allows a ttl of 1.2 hours", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: 1.2,
    };
    const validate = () => validationSchema.validateSync(values);
    expect(validate).not.toThrowError();
  });
});

describe("ttlShutdownAt", () => {
  it.each<[string, number, string]>([
    [
      "Manual shutdown --> manual helper text",
      0,
      "Your workspace will not automatically shut down.",
    ],
    [
      "One hour --> helper text shows shutdown after 1 hour",
      1,
      `Your workspace will shut down 1 hour after its next start. We delay shutdown by 1 hour whenever we detect activity.`,
    ],
    [
      "Two hours --> helper text shows shutdown after 2 hours",
      2,
      `Your workspace will shut down 2 hours after its next start. We delay shutdown by 1 hour whenever we detect activity.`,
    ],
    [
      "24 hours --> helper text shows shutdown after 1 day",
      24,
      `Your workspace will shut down 1 day after its next start. We delay shutdown by 1 hour whenever we detect activity.`,
    ],
    [
      "48 hours --> helper text shows shutdown after 2 days",
      48,
      `Your workspace will shut down 2 days after its next start. We delay shutdown by 1 hour whenever we detect activity.`,
    ],
    [
      "1.2 hours --> helper text shows shutdown after 1 hour and 12 minutes",
      1.2,
      `Your workspace will shut down 1 hour and 12 minutes after its next start. We delay shutdown by 1 hour whenever we detect activity.`,
    ],
    [
      "24.2 hours --> helper text shows shutdown after 1 day and 12 minutes",
      24.2,
      `Your workspace will shut down 1 day and 12 minutes after its next start. We delay shutdown by 1 hour whenever we detect activity.`,
    ],
    [
      "0.2 hours --> helper text shows shutdown after 12 minutes",
      0.2,
      `Your workspace will shut down 12 minutes after its next start. We delay shutdown by 1 hour whenever we detect activity.`,
    ],
    [
      "48.258 hours --> helper text shows shutdown after 2 days and 15 minutes and 28 seconds",
      48.258,
      `Your workspace will shut down 2 days and 15 minutes and 28 seconds after its next start. We delay shutdown by 1 hour whenever we detect activity.`,
    ],
  ])("%p", (_, ttlHours, expected) => {
    expect(ttlShutdownAt(ttlHours)).toEqual(expected);
  });
});

const autoStartDayLabels = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const defaultFormProps = {
  submitScheduleError: "",
  initialValues: {
    ...defaultSchedule(),
    autostartEnabled: true,
    autostopEnabled: true,
    ttl: 24,
  },
  isLoading: false,
  defaultTTL: 24,
  onCancel: () => null,
  onSubmit: () => null,
  allowedTemplateAutoStartDays: autoStartDayLabels,
  allowTemplateAutoStart: true,
  allowTemplateAutoStop: true,
};

describe("templateInheritance", () => {
  it("disables the entire autostart feature appropriately", async () => {
    jest.spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
    render(
      <WorkspaceScheduleForm
        {...defaultFormProps}
        allowTemplateAutoStart={false}
      />,
    );

    const autoStartToggle = await screen.findByLabelText("Enable Autostart");
    expect(autoStartToggle).toBeDisabled();

    const startTimeInput = await screen.findByLabelText("Start time");
    expect(startTimeInput).toBeDisabled();

    const timezoneInput = await screen.findByLabelText("Timezone");
    // MUI's input is wrapped in a div so we look at the aria-attribute instead
    expect(timezoneInput).toHaveAttribute("aria-disabled");

    for (const label of autoStartDayLabels) {
      const checkbox = await screen.findByLabelText(label);
      expect(checkbox).toBeDisabled();
    }
  });
  it("disables the autostart days of the week appropriately", async () => {
    const enabledDayLabels = ["Sat", "Sun"];

    jest.spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
    render(
      <WorkspaceScheduleForm
        {...defaultFormProps}
        allowedTemplateAutoStartDays={["saturday", "sunday"]}
      />,
    );

    const autoStartToggle = await screen.findByLabelText("Enable Autostart");
    expect(autoStartToggle).toBeEnabled();

    const startTimeInput = await screen.findByLabelText("Start time");
    expect(startTimeInput).toBeEnabled();

    const timezoneInput = await screen.findByLabelText("Timezone");
    // MUI's input is wrapped in a div so we look at the aria-attribute instead
    expect(timezoneInput).not.toHaveAttribute("aria-disabled");

    for (const label of enabledDayLabels) {
      const checkbox = await screen.findByLabelText(label);
      expect(checkbox).toBeEnabled();
    }

    for (const label of autoStartDayLabels.filter(
      (day) => !enabledDayLabels.includes(day),
    )) {
      const checkbox = await screen.findByLabelText(label);
      expect(checkbox).toBeDisabled();
    }
  });
  it("disables the entire autostop feature appropriately", async () => {
    jest.spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
    render(
      <WorkspaceScheduleForm
        {...defaultFormProps}
        allowTemplateAutoStop={false}
      />,
    );

    const autoStopToggle = await screen.findByLabelText("Enable Autostop");
    expect(autoStopToggle).toBeDisabled();

    const ttlInput = await screen.findByLabelText(
      "Time until shutdown (hours)",
    );
    expect(ttlInput).toBeDisabled();
  });
  it("disables secondary autostart fields if main feature switch is toggled off", async () => {
    jest.spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
    render(
      <WorkspaceScheduleForm
        {...defaultFormProps}
        initialValues={{
          ...defaultFormProps.initialValues,
          autostartEnabled: false,
        }}
      />,
    );

    const startTimeInput = await screen.findByLabelText("Start time");
    expect(startTimeInput).toBeDisabled();

    const timezoneInput = await screen.findByLabelText("Timezone");
    // MUI's input is wrapped in a div so we look at the aria-attribute instead
    expect(timezoneInput).toHaveAttribute("aria-disabled");

    autoStartDayLabels.forEach(async (label) => {
      const checkbox = await screen.findByLabelText(label);
      expect(checkbox).toBeDisabled();
    });
  });
  it("disables secondary autostop fields if main feature switch is toggled off", async () => {
    jest.spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
    render(
      <WorkspaceScheduleForm
        {...defaultFormProps}
        initialValues={{
          ...defaultFormProps.initialValues,
          autostopEnabled: false,
        }}
      />,
    );

    const ttlInput = await screen.findByLabelText(
      "Time until shutdown (hours)",
    );
    expect(ttlInput).toBeDisabled();
  });
});

test("form should be enabled when both auto stop and auto start features are disabled, given that the template permits these actions", async () => {
  jest.spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
  render(
    <WorkspaceScheduleForm
      {...defaultFormProps}
      initialValues={{
        ...defaultFormProps.initialValues,
        autostopEnabled: false,
        autostartEnabled: false,
      }}
    />,
  );

  const submitButton = await screen.findByRole("button", { name: "Submit" });
  expect(submitButton).toBeEnabled();
});

test("form should be disabled when both auto stop and auto start features are disabled at template level", async () => {
  jest.spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
  render(
    <WorkspaceScheduleForm
      {...defaultFormProps}
      allowTemplateAutoStart={false}
      allowTemplateAutoStop={false}
      initialValues={{
        ...defaultFormProps.initialValues,
      }}
    />,
  );

  const submitButton = await screen.findByRole("button", { name: "Submit" });
  expect(submitButton).toBeDisabled();
});
