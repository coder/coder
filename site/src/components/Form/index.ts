import { FormikContextType } from "formik/dist/types"

export * from "./FormCloseButton"
export * from "./FormSection"
export * from "./FormDropdownField"
export * from "./FormTextField"
export * from "./FormTitle"

export  function getFormHelpers<T>(form: FormikContextType<T>, name: keyof T) {
    const touched = form.touched[name]
    const errors = form.errors[name]
    return {
      ...form.getFieldProps(name),
      id: name,
      error: touched && Boolean(errors),
      helperText: touched && errors
    }
  }
