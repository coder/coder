import axios from "axios"
import * as Types from "./types"

export const Language = {
  errorsByCode: {
    default: "Invalid value",
    exists: "This value is already in use",
  },
}

const getApiError = (error: unknown): Types.ApiError | undefined => {
  if (axios.isAxiosError(error)) {
    return error.response?.data
  }
}

export const getFormErrorsFromApiError = (error: unknown): Record<string, string> | undefined => {
  const apiError = getApiError(error)

  if (apiError && apiError.errors) {
    return apiError.errors.reduce((errors, error) => {
      return {
        ...errors,
        [error.field]:
          error.code in Language.errorsByCode
            ? Language.errorsByCode[error.code as keyof typeof Language.errorsByCode]
            : Language.errorsByCode.default,
      }
    }, {})
  }
}
