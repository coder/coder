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
      ttl: 0,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrow()
  })

  it("disallows ttl to be negative", () => {
    const values = {
      ...valid,
      ttl: -1,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrow()
  })

  it("disallows all days-of-week to be false when startTime is set", () => {
    const values = {
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
})
