import { hasApiFieldErrors, isApiError, mapApiErrorToFieldErrors } from "api/errors"
import { FormikContextType, FormikErrors, getIn } from "formik"
import { ChangeEvent, ChangeEventHandler, FocusEventHandler, ReactNode } from "react"
import * as Yup from "yup"

export const Language = {
  nameRequired: (name: string): string => {
    return `Please enter a ${name.toLowerCase()}.`
  },
  nameInvalidChars: (name: string): string => {
    return `${name} must start with a-Z or 0-9 and can contain a-Z, 0-9 or -`
  },
  nameTooLong: (name: string): string => {
    return `${name} cannot be longer than 32 characters`
  },
}

interface FormHelpers {
  name: string
  onBlur: FocusEventHandler
  onChange: ChangeEventHandler
  id: string
  value?: string | number
  error: boolean
  helperText?: ReactNode
}

// backendErrorName can be used if the backend names a field differently than the frontend does
export const getFormHelpers =
  <T>(form: FormikContextType<T>, apiValidationErrors?: FormikErrors<T>) =>
  (name: keyof T, HelperText: ReactNode = "", backendErrorName?: string): FormHelpers => {
    if (typeof name !== "string") {
      throw new Error(`name must be type of string, instead received '${typeof name}'`)
    }
    const apiErrorName = backendErrorName ?? name

    // getIn is a util function from Formik that gets at any depth of nesting
    // and is necessary for the types to work
    const touched = getIn(form.touched, name)
    const apiError = getIn(apiValidationErrors, apiErrorName)
    const frontendError = getIn(form.errors, name)
    const error = apiError ?? frontendError
    return {
      ...form.getFieldProps(name),
      id: name,
      error: touched && Boolean(error),
      helperText: touched ? error || HelperText : HelperText,
    }
  }

export const getFormHelpersWithError = <T>(
  form: FormikContextType<T>,
  error?: Error | unknown,
): ((name: keyof T, HelperText?: ReactNode, errorName?: string) => FormHelpers) => {
  const apiValidationErrors =
    isApiError(error) && hasApiFieldErrors(error)
      ? (mapApiErrorToFieldErrors(error.response.data) as FormikErrors<T>)
      : undefined
  return getFormHelpers(form, apiValidationErrors)
}

export const onChangeTrimmed =
  <T>(form: FormikContextType<T>) =>
  (event: ChangeEvent<HTMLInputElement>): void => {
    event.target.value = event.target.value.trim()
    form.handleChange(event)
  }

// REMARK: Keep in sync with coderd/httpapi/httpapi.go#L40
const maxLenName = 32

// REMARK: Keep in sync with coderd/httpapi/httpapi.go#L18
const usernameRE = /^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$/

// REMARK: see #1756 for name/username semantics
export const nameValidator = (name: string): Yup.StringSchema =>
  Yup.string()
    .required(Language.nameRequired(name))
    .matches(usernameRE, Language.nameInvalidChars(name))
    .max(maxLenName, Language.nameTooLong(name))
