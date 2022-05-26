import { Language, validationSchema, WorkspaceScheduleFormValues } from "./WorkspaceScheduleForm"

const valid: WorkspaceScheduleFormValues = {
  sunday: false,
  monday: true,
  tuesday: true,
  wednesday: true,
  thursday: true,
  friday: true,
  saturday: false,

  startTime: "09:30",
  timezone: "Canada/Eastern",
  ttl: 120,
}

describe("validationSchema", () => {
  it("allows everything to be falsy", () => {
    const values: WorkspaceScheduleFormValues = {
      sunday: false,
      monday: false,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,

      startTime: "",
      timezone: "",
      ttl: 0,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrow()
  })

  it("disallows ttl to be negative", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: -1,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrow()
  })

  it("disallows all days-of-week to be false when startTime is set", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      sunday: false,
      monday: false,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorNoDayOfWeek)
  })

  it("allows startTime 16:20", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "16:20",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrow()
  })

  it("disallows startTime to be H:mm", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "9:30",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTime)
  })

  it("disallows startTime to be HH:m", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "09:5",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTime)
  })

  it("disallows an invalid startTime 24:01", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "24:01",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTime)
  })

  it("disallows an invalid startTime 09:60", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "09:60",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTime)
  })

  it("disallows an invalid timezone Canada/North", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      timezone: "Canada/North",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTimezone)
  })
})
