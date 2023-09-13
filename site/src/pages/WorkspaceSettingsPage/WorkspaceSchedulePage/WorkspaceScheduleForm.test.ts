import {
  Language,
  ttlShutdownAt,
  validationSchema,
  WorkspaceScheduleFormValues,
} from "./WorkspaceScheduleForm";
import { zones } from "./zones";

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

  it.each<[string]>(zones.map((zone) => [zone]))(
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
});

describe("ttlShutdownAt", () => {
  it.each<[string, number, string]>([
    [
      "Manual shutdown --> manual helper text",
      0,
      Language.ttlCausesNoShutdownHelperText,
    ],
    [
      "One hour --> helper text shows shutdown after an hour",
      1,
      `${Language.ttlCausesShutdownHelperText} an hour ${Language.ttlCausesShutdownAfterStart}.`,
    ],
    [
      "Two hours --> helper text shows shutdown after 2 hours",
      2,
      `${Language.ttlCausesShutdownHelperText} 2 hours ${Language.ttlCausesShutdownAfterStart}.`,
    ],
    [
      "24 hours --> helper text shows shutdown after a day",
      24,
      `${Language.ttlCausesShutdownHelperText} a day ${Language.ttlCausesShutdownAfterStart}.`,
    ],
    [
      "48 hours --> helper text shows shutdown after 2 days",
      48,
      `${Language.ttlCausesShutdownHelperText} 2 days ${Language.ttlCausesShutdownAfterStart}.`,
    ],
  ])("%p", (_, ttlHours, expected) => {
    expect(ttlShutdownAt(ttlHours)).toEqual(expected);
  });
});
