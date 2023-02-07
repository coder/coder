import { isApiValidationError, mapApiErrorToFieldErrors } from "api/errors"
import { FormikContextType, FormikErrors, getIn } from "formik"
import {
  ChangeEvent,
  ChangeEventHandler,
  FocusEventHandler,
  ReactNode,
} from "react"
import * as Yup from "yup"

const Language = {
  nameRequired: (name: string): string => {
    return name ? `Please enter a ${name.toLowerCase()}.` : "Required"
  },
  nameInvalidChars: (name: string): string => {
    return `${name} must start with a-Z or 0-9 and can contain a-Z, 0-9 or -`
  },
  nameTooLong: (name: string, len: number): string => {
    return `${name} cannot be longer than ${len} characters`
  },
  templateDisplayNameInvalidChars: (name: string): string => {
    return `${name} must start and end with non-whitespace character`
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

export const getFormHelpers =
  <T>(form: FormikContextType<T>, error?: Error | unknown) =>
  (
    name: string,
    HelperText: ReactNode = "",
    backendErrorName?: string,
  ): FormHelpers => {
    const apiValidationErrors = isApiValidationError(error)
      ? (mapApiErrorToFieldErrors(error.response.data) as FormikErrors<T>)
      : // This should not return the error since it is not and api validation error but I didn't have time to fix this and tests
        error

    if (typeof name !== "string") {
      throw new Error(
        `name must be type of string, instead received '${typeof name}'`,
      )
    }

    const apiErrorName = backendErrorName ?? name

    // getIn is a util function from Formik that gets at any depth of nesting
    // and is necessary for the types to work
    const touched = getIn(form.touched, name)
    const apiError = getIn(apiValidationErrors, apiErrorName)
    const frontendError = getIn(form.errors, name)
    const returnError = apiError ?? frontendError

    return {
      ...form.getFieldProps(name),
      id: name,
      error: touched && Boolean(returnError),
      helperText: touched ? returnError || HelperText : HelperText,
    }
  }

export const onChangeTrimmed =
  <T>(form: FormikContextType<T>) =>
  (event: ChangeEvent<HTMLInputElement>): void => {
    event.target.value = event.target.value.trim()
    form.handleChange(event)
  }

// REMARK: Keep these consts in sync with coderd/httpapi/httpapi.go
const maxLenName = 32
const templateDisplayNameMaxLength = 64
const usernameRE = /^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$/
const templateDisplayNameRE = /^[^\s](.*[^\s])?$/

// REMARK: see #1756 for name/username semantics
export const nameValidator = (name: string): Yup.StringSchema =>
  Yup.string()
    .required(Language.nameRequired(name))
    .matches(usernameRE, Language.nameInvalidChars(name))
    .max(maxLenName, Language.nameTooLong(name, maxLenName))

export const templateDisplayNameValidator = (
  displayName: string,
): Yup.StringSchema =>
  Yup.string()
    .matches(
      templateDisplayNameRE,
      Language.templateDisplayNameInvalidChars(displayName),
    )
    .max(
      templateDisplayNameMaxLength,
      Language.nameTooLong(displayName, templateDisplayNameMaxLength),
    )
