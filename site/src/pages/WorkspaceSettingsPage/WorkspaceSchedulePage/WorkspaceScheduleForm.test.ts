import {
  Language,
  ttlShutdownAt,
  validationSchema,
  WorkspaceScheduleFormValues,
} from "./WorkspaceScheduleForm";
import { timeZones } from "utils/timeZones";

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
      `Your workspace will shut down 1 hour after its next start. We delay shutdown by this time whenever we detect activity.`,
    ],
    [
      "Two hours --> helper text shows shutdown after 2 hours",
      2,
      `Your workspace will shut down 2 hours after its next start. We delay shutdown by this time whenever we detect activity.`,
    ],
    [
      "24 hours --> helper text shows shutdown after 1 day",
      24,
      `Your workspace will shut down 1 day after its next start. We delay shutdown by this time whenever we detect activity.`,
    ],
    [
      "48 hours --> helper text shows shutdown after 2 days",
      48,
      `Your workspace will shut down 2 days after its next start. We delay shutdown by this time whenever we detect activity.`,
    ],
    [
      "1.2 hours --> helper text shows shutdown after 1 hour and 12 minutes",
      1.2,
      `Your workspace will shut down 1 hour and 12 minutes after its next start. We delay shutdown by this time whenever we detect activity.`,
    ],
    [
      "24.2 hours --> helper text shows shutdown after 1 day and 12 minutes",
      24.2,
      `Your workspace will shut down 1 day and 12 minutes after its next start. We delay shutdown by this time whenever we detect activity.`,
    ],
    [
      "0.2 hours --> helper text shows shutdown after 12 minutes",
      0.2,
      `Your workspace will shut down 12 minutes after its next start. We delay shutdown by this time whenever we detect activity.`,
    ],
    [
      "48.258 hours --> helper text shows shutdown after 2 days and 15 minutes and 28 seconds",
      48.258,
      `Your workspace will shut down 2 days and 15 minutes and 28 seconds after its next start. We delay shutdown by this time whenever we detect activity.`,
    ],
  ])("%p", (_, ttlHours, expected) => {
    expect(ttlShutdownAt(ttlHours)).toEqual(expected);
  });
});
