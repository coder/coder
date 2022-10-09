import {
  getValidationErrorMessage,
  isApiError,
  mapApiErrorToFieldErrors,
} from "./errors"

describe("isApiError", () => {
  it("returns true when the object is an API Error", () => {
    expect(
      isApiError({
        isAxiosError: true,
        response: {
          data: {
            message: "Invalid entry",
            errors: [
              { detail: "Username is already in use", field: "username" },
            ],
          },
        },
      }),
    ).toBe(true)
  })

  it("returns false when the object is Error", () => {
    expect(isApiError(new Error())).toBe(false)
  })

  it("returns false when the object is undefined", () => {
    expect(isApiError(undefined)).toBe(false)
  })
})

describe("mapApiErrorToFieldErrors", () => {
  it("returns correct field errors", () => {
    expect(
      mapApiErrorToFieldErrors({
        message: "Invalid entry",
        validations: [
          { detail: "Username is already in use", field: "username" },
        ],
      }),
    ).toEqual({
      username: "Username is already in use",
    })
  })
})

describe("getValidationErrorMessage", () => {
  it("returns multiple validation messages", () => {
    expect(
      getValidationErrorMessage({
        response: {
          data: {
            message: "Invalid user search query.",
            validations: [
              {
                field: "status",
                detail: `Query param "status" has invalid value: "inactive" is not a valid user status`,
              },
              {
                field: "q",
                detail: `Query element "role:a:e" can only contain 1 ':'`,
              },
            ],
          },
        },
        isAxiosError: true,
      }),
    ).toEqual(
      `Query param "status" has invalid value: "inactive" is not a valid user status\nQuery element "role:a:e" can only contain 1 ':'`,
    )
  })

  it("non-API error returns empty validation message", () => {
    expect(
      getValidationErrorMessage({
        response: {
          data: {
            error: "Invalid user search query.",
          },
        },
        isAxiosError: true,
      }),
    ).toEqual("")
  })

  it("no validations field returns empty validation message", () => {
    expect(
      getValidationErrorMessage({
        response: {
          data: {
            message: "Invalid user search query.",
            detail: `Query element "role:a:e" can only contain 1 ':'`,
          },
        },
        isAxiosError: true,
      }),
    ).toEqual("")
  })
})
