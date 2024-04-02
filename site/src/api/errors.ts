import axios, { type AxiosError, type AxiosResponse } from "axios";

const Language = {
  errorsByCode: {
    defaultErrorCode: "Invalid value",
  },
};

export interface FieldError {
  field: string;
  detail: string;
}

export type FieldErrors = Record<FieldError["field"], FieldError["detail"]>;

export interface ApiErrorResponse {
  message: string;
  detail?: string;
  validations?: FieldError[];
}

export type ApiError = AxiosError<ApiErrorResponse> & {
  response: AxiosResponse<ApiErrorResponse>;
};

export const isApiError = (err: unknown): err is ApiError => {
  return (
    axios.isAxiosError(err) &&
    err.response !== undefined &&
    isApiErrorResponse(err.response.data)
  );
};

export const isApiErrorResponse = (err: unknown): err is ApiErrorResponse => {
  return (
    typeof err === "object" &&
    err !== null &&
    "message" in err &&
    typeof err.message === "string" &&
    (!("detail" in err) ||
      err.detail === undefined ||
      typeof err.detail === "string") &&
    (!("validations" in err) ||
      err.validations === undefined ||
      Array.isArray(err.validations))
  );
};

export const hasApiFieldErrors = (error: ApiError): boolean =>
  Array.isArray(error.response.data.validations);

export const isApiValidationError = (error: unknown): error is ApiError => {
  return isApiError(error) && hasApiFieldErrors(error);
};

export const hasError = (error: unknown) =>
  error !== undefined && error !== null;

export const mapApiErrorToFieldErrors = (
  apiErrorResponse: ApiErrorResponse,
): FieldErrors => {
  const result: FieldErrors = {};

  if (apiErrorResponse.validations) {
    for (const error of apiErrorResponse.validations) {
      result[error.field] =
        error.detail || Language.errorsByCode.defaultErrorCode;
    }
  }

  return result;
};

/**
 *
 * @param error
 * @param defaultMessage
 * @returns error's message if ApiError or Error, else defaultMessage
 */
export const getErrorMessage = (
  error: unknown,
  defaultMessage: string,
): string => {
  // if error is API error
  // 404s result in the default message being returned
  if (isApiError(error) && error.response.data.message) {
    return error.response.data.message;
  }
  if (isApiErrorResponse(error)) {
    return error.message;
  }
  // if error is a non-empty string
  if (error && typeof error === "string") {
    return error;
  }
  return defaultMessage;
};

/**
 *
 * @param error
 * @returns a combined validation error message if the error is an ApiError
 * and contains validation messages for different form fields.
 */
export const getValidationErrorMessage = (error: unknown): string => {
  const validationErrors =
    isApiError(error) && error.response.data.validations
      ? error.response.data.validations
      : [];
  return validationErrors.map((error) => error.detail).join("\n");
};

export const getErrorDetail = (error: unknown): string | undefined | null => {
  if (error instanceof Error) {
    return "Please check the developer console for more details.";
  }
  if (isApiError(error)) {
    return error.response.data.detail;
  }
  if (isApiErrorResponse(error)) {
    return error.detail;
  }
  return null;
};
