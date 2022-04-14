import axios from "axios"

export const Language = {
  errorsByCode: {
    defaultErrorCode: "Invalid value",
    exists: "This value is already in use",
  },
}

interface FieldError {
  field: string
  code: string
}

type FieldErrors = Record<FieldError["field"], FieldError["code"]>

export interface ApiError {
  message: string
  errors?: FieldError[]
}

export const mapApiErrorToFieldErrors = (apiError: ApiError): FieldErrors => {
  const result: FieldErrors = {}

  if (apiError.errors) {
    for (const error of apiError.errors) {
      result[error.field] = error.code || Language.errorsByCode.defaultErrorCode
    }
  }

  return result
}

export const getApiError = (error: unknown): ApiError | undefined => {
  if (axios.isAxiosError(error)) {
    return error.response?.data
  }
}
