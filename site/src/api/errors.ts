import axios, { AxiosError, AxiosResponse } from "axios"

const Language = {
  errorsByCode: {
    defaultErrorCode: "Invalid value",
  },
}

export interface FieldError {
  field: string
  detail: string
}

export type FieldErrors = Record<FieldError["field"], FieldError["detail"]>

export interface ApiErrorResponse {
  message: string
  detail?: string
  validations?: FieldError[]
}

export type ApiError = AxiosError<ApiErrorResponse> & {
  response: AxiosResponse<ApiErrorResponse>
}

export const isApiError = (err: unknown): err is ApiError => {
  if (axios.isAxiosError(err)) {
    const response = err.response?.data
    if (!response) {
      return false
    }

    return (
      typeof response.message === "string" &&
      (typeof response.errors === "undefined" || Array.isArray(response.errors))
    )
  }

  return false
}

/**
 * ApiErrors contain useful error messages in their response body. They contain an overall message
 * and may also contain errors for specific form fields.
 * @param error ApiError
 * @returns true if the ApiError contains error messages for specific form fields.
 */
export const hasApiFieldErrors = (error: ApiError): boolean =>
  Array.isArray(error.response.data.validations)

export const mapApiErrorToFieldErrors = (
  apiErrorResponse: ApiErrorResponse,
): FieldErrors => {
  const result: FieldErrors = {}

  if (apiErrorResponse.validations) {
    for (const error of apiErrorResponse.validations) {
      result[error.field] =
        error.detail || Language.errorsByCode.defaultErrorCode
    }
  }

  return result
}

export const isApiValidationError = (error: unknown): error is ApiError => {
  return isApiError(error) && hasApiFieldErrors(error)
}

/**
 *
 * @param error
 * @param defaultMessage
 * @returns error's message if ApiError or Error, else defaultMessage
 */
export const getErrorMessage = (
  error: Error | ApiError | unknown,
  defaultMessage: string,
): string =>
  isApiError(error)
    ? error.response.data.message
    : error instanceof Error
    ? error.message
    : defaultMessage

/**
 *
 * @param error
 * @returns a combined validation error message if the error is an ApiError
 * and contains validation messages for different form fields.
 */
export const getValidationErrorMessage = (
  error: Error | ApiError | unknown,
): string => {
  const validationErrors =
    isApiError(error) && error.response.data.validations
      ? error.response.data.validations
      : []
  return validationErrors.map((error) => error.detail).join("\n")
}

export const getErrorDetail = (
  error: Error | ApiError | unknown,
): string | undefined | null =>
  isApiError(error)
    ? error.response.data.detail
    : error instanceof Error
    ? error.stack
    : null
