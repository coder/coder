import axios, { AxiosError, AxiosResponse } from "axios"

export const Language = {
  errorsByCode: {
    defaultErrorCode: "Invalid value",
  },
}

interface FieldError {
  field: string
  detail: string
}

export type FieldErrors = Record<FieldError["field"], FieldError["detail"]>

export interface ApiErrorResponse {
  message: string
  errors?: FieldError[]
}

export type ApiError = AxiosError<ApiErrorResponse> & { response: AxiosResponse<ApiErrorResponse> }

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types, @typescript-eslint/no-explicit-any
export const isApiError = (err: any): err is ApiError => {
  if (axios.isAxiosError(err)) {
    const response = err.response?.data

    return (
      typeof response.message === "string" && (typeof response.errors === "undefined" || Array.isArray(response.errors))
    )
  }

  return false
}

export const mapApiErrorToFieldErrors = (apiErrorResponse: ApiErrorResponse): FieldErrors => {
  const result: FieldErrors = {}

  if (apiErrorResponse.errors) {
    for (const error of apiErrorResponse.errors) {
      result[error.field] = error.detail || Language.errorsByCode.defaultErrorCode
    }
  }

  return result
}
