import { FormikLike } from "../../util/formik"

/**
 * FormFieldProps are required props for creating form fields using a factory.
 */
export interface FormFieldProps<T> {
  /**
   * form is a reference to a form or subform and is used to compute common
   * states such as error and helper text
   */
  form: FormikLike<T>
  /**
   * formFieldName is a field name associated with the form schema.
   */
  formFieldName: keyof T
}
