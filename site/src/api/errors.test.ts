import { mockApiError } from "testHelpers/entities";
import {
  getValidationErrorMessage,
  isApiError,
  mapApiErrorToFieldErrors,
  getErrorMessage,
} from "./errors";

describe("isApiError", () => {
  it("returns true when the object is an API Error", () => {
    expect(
      isApiError(
        mockApiError({
          message: "Invalid entry",
          validations: [
            { detail: "Username is already in use", field: "username" },
          ],
        }),
      ),
    ).toBe(true);
  });

  it("returns false when the object is Error", () => {
    expect(isApiError(new Error())).toBe(false);
  });

  it("returns false when the object is undefined", () => {
    expect(isApiError(undefined)).toBe(false);
  });
});

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
    });
  });
});

describe("getValidationErrorMessage", () => {
  it("returns multiple validation messages", () => {
    expect(
      getValidationErrorMessage(
        mockApiError({
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
        }),
      ),
    ).toEqual(
      `Query param "status" has invalid value: "inactive" is not a valid user status\nQuery element "role:a:e" can only contain 1 ':'`,
    );
  });

  it("non-API error returns empty validation message", () => {
    expect(
      getValidationErrorMessage(new Error("Invalid user search query.")),
    ).toEqual("");
  });

  it("no validations field returns empty validation message", () => {
    expect(
      getValidationErrorMessage(
        mockApiError({
          message: "Invalid user search query.",
          detail: `Query element "role:a:e" can only contain 1 ':'`,
        }),
      ),
    ).toEqual("");
  });

  it("returns default message for error that is empty string", () => {
    expect(getErrorMessage("", "Something went wrong.")).toBe(
      "Something went wrong.",
    );
  });

  it("returns default message for 404 API response", () => {
    expect(
      getErrorMessage(
        mockApiError({
          message: "",
        }),
        "Something went wrong.",
      ),
    ).toBe("Something went wrong.");
  });
});
