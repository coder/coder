import { isApiError, mapApiErrorToFieldErrors } from "./errors"

describe("isApiError", () => {
  it("returns true when the object is an API Error", () => {
    expect(
      isApiError({
        isAxiosError: true,
        response: {
          data: {
            message: "Invalid entry",
            errors: [{ detail: "Username is already in use", field: "username" }],
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
        validations: [{ detail: "Username is already in use", field: "username" }],
      }),
    ).toEqual({
      username: "Username is already in use",
    })
  })
})
