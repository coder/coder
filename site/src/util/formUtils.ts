import { FormikContextType, FormikErrors, getIn } from "formik"
import { ChangeEvent, ChangeEventHandler, FocusEventHandler } from "react"

interface FormHelpers {
  name: string
  onBlur: FocusEventHandler
  onChange: ChangeEventHandler
  id: string
  value?: string | number
  error: boolean
  helperText?: string
}

export const getFormHelpers =
  <T>(form: FormikContextType<T>, formErrors?: FormikErrors<T>) =>
  (name: keyof T): FormHelpers => {
    if (typeof name !== "string") {
      throw new Error(`name must be type of string, instead received '${typeof name}'`)
    }

    // getIn is a util function from Formik that gets at any depth of nesting
    // and is necessary for the types to work
    const touched = getIn(form.touched, name)
    const apiError = getIn(formErrors, name)
    const validationError = getIn(form.errors, name)
    const error = apiError ?? validationError
    return {
      ...form.getFieldProps(name),
      id: name,
      error: touched && Boolean(error),
      helperText: touched && error,
    }
  }

export const onChangeTrimmed =
  <T>(form: FormikContextType<T>) =>
  (event: ChangeEvent<HTMLInputElement>): void => {
    event.target.value = event.target.value.trim()
    form.handleChange(event)
  }
