/* eslint-disable @typescript-eslint/no-explicit-any */
/* eslint-disable @typescript-eslint/explicit-module-boundary-types */
import axios from "axios"

export const Language = {
  errorsByCode: {
    defaultErrorCode: "Invalid value",
  },
}

interface FieldError {
  field: string
  detail: string
}

type FieldErrors = Record<FieldError["field"], FieldError["detail"]>

export interface ApiError {
  message: string
  errors?: FieldError[]
}

const unwrapAxiosError = (obj: unknown): unknown => {
  if (axios.isAxiosError(obj)) {
    return obj.response?.data
  } else {
    return obj
  }
}

export const isApiError = (err: any): err is ApiError => {
  const maybeApiError = unwrapAxiosError(err) as Partial<ApiError> | undefined

  if (!maybeApiError || maybeApiError instanceof Error) {
    return false
  } else if (typeof maybeApiError.message === "string") {
    return typeof maybeApiError.errors === "undefined" || Array.isArray(maybeApiError.errors)
  } else {
    return false
  }
}

export const mapApiErrorToFieldErrors = (apiError: ApiError): FieldErrors => {
  const result: FieldErrors = {}

  if (apiError.errors) {
    for (const error of apiError.errors) {
      result[error.field] = error.detail || Language.errorsByCode.defaultErrorCode
    }
  }

  return result
}
