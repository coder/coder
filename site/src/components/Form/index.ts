import { FormikContextType, getIn } from "formik"
import { ChangeEvent, ChangeEventHandler, FocusEventHandler } from "react"

export * from "./FormCloseButton"
export * from "./FormDropdownField"
export * from "./FormSection"
export * from "./FormTextField"
export * from "./FormTitle"

interface FormHelpers {
  name: string
  onBlur: FocusEventHandler
  onChange: ChangeEventHandler
  id: string
  value?: string | number
  error: boolean
  helperText?: string
}

export const getFormHelpers = <T>(form: FormikContextType<T>, name: string, error?: string): FormHelpers => {
  // getIn is a util function from Formik that gets at any depth of nesting, and is necessary for the types to work
  const touched = getIn(form.touched, name)
  const errors = error ?? getIn(form.errors, name)
  return {
    ...form.getFieldProps(name),
    id: name,
    error: touched && Boolean(errors),
    helperText: touched && errors,
  }
}

export const onChangeTrimmed =
  <T>(form: FormikContextType<T>) =>
  (event: ChangeEvent<HTMLInputElement>): void => {
    event.target.value = event.target.value.trim()
    form.handleChange(event)
  }
