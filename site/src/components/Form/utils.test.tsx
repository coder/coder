import { FormikContextType } from "formik/dist/types"
import { getFormHelpers, onChangeTrimmed } from "./utils"

interface TestType {
  untouchedGoodField: string
  untouchedBadField: string
  touchedGoodField: string
  touchedBadField: string
}

const mockHandleChange = jest.fn()

const form = {
  errors: {
    untouchedGoodField: undefined,
    untouchedBadField: "oops!",
    touchedGoodField: undefined,
    touchedBadField: "oops!",
  },
  touched: {
    untouchedGoodField: false,
    untouchedBadField: false,
    touchedGoodField: true,
    touchedBadField: true,
  },
  handleChange: mockHandleChange,
  handleBlur: jest.fn(),
  getFieldProps: (name: string) => {
    return {
      name,
      onBlur: jest.fn(),
      onChange: jest.fn(),
      value: "",
    }
  },
} as unknown as FormikContextType<TestType>

describe("form util functions", () => {
  describe("getFormHelpers", () => {
    const untouchedGoodResult = getFormHelpers<TestType>(form, "untouchedGoodField")
    const untouchedBadResult = getFormHelpers<TestType>(form, "untouchedBadField")
    const touchedGoodResult = getFormHelpers<TestType>(form, "touchedGoodField")
    const touchedBadResult = getFormHelpers<TestType>(form, "touchedBadField")
    it("populates the 'field props'", () => {
      expect(untouchedGoodResult.name).toEqual("untouchedGoodField")
      expect(untouchedGoodResult.onBlur).toBeDefined()
      expect(untouchedGoodResult.onChange).toBeDefined()
      expect(untouchedGoodResult.value).toBeDefined()
    })
    it("sets the id to the name", () => {
      expect(untouchedGoodResult.id).toEqual("untouchedGoodField")
    })
    it("sets error to true if touched and invalid", () => {
      expect(untouchedGoodResult.error).toBeFalsy
      expect(untouchedBadResult.error).toBeFalsy
      expect(touchedGoodResult.error).toBeFalsy
      expect(touchedBadResult.error).toBeTruthy
    })
    it("sets helperText to the error message if touched and invalid", () => {
      expect(untouchedGoodResult.helperText).toBeUndefined
      expect(untouchedBadResult.helperText).toBeUndefined
      expect(touchedGoodResult.helperText).toBeUndefined
      expect(touchedBadResult.helperText).toEqual("oops!")
    })
  })

  describe("onChangeTrimmed", () => {
    it("calls handleChange with trimmed value", () => {
      const event = { target: { value: " hello " } } as React.ChangeEvent<HTMLInputElement>
      onChangeTrimmed<TestType>(form)(event)
      expect(mockHandleChange).toHaveBeenCalledWith({ target: { value: "hello" } })
    })
  })
})
