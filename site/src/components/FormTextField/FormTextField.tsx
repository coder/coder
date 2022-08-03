import TextField, { TextFieldProps } from "@material-ui/core/TextField"
import { FormikContextType } from "formik"
import { ReactElement } from "react"
import { PasswordField } from "../PasswordField/PasswordField"

/**
 * FormFieldProps are required props for creating form fields using a factory.
 */
export interface FormFieldProps<T> {
  /**
   * form is a reference to a form or subform and is used to compute common
   * states such as error and helper text
   */
  form: FormikContextType<T>
  /**
   * formFieldName is a field name associated with the form schema.
   */
  formFieldName: keyof T
}

/**
 * FormTextFieldProps extends form-related MUI TextFieldProps with Formik
 * props. The passed in form is used to compute error states and configure
 * change handlers. `formFieldName` represents the key of a Formik value
 * that's associated to this component.
 */
export interface FormTextFieldProps<T>
  extends Pick<
      TextFieldProps,
      | "autoComplete"
      | "autoFocus"
      | "children"
      | "className"
      | "disabled"
      | "fullWidth"
      | "helperText"
      | "id"
      | "InputLabelProps"
      | "InputProps"
      | "inputProps"
      | "label"
      | "margin"
      | "multiline"
      | "onChange"
      | "placeholder"
      | "required"
      | "rows"
      | "select"
      | "SelectProps"
      | "style"
      | "type"
    >,
    FormFieldProps<T> {
  /**
   * eventTransform is an optional transformer on the event data before it is
   * processed by formik.
   *
   * @example
   * <FormTextField
   *   eventTransformer={(str) => {
   *     return str.replace(" ", "-")
   *   }}
   * />
   */
  eventTransform?: (value: string) => string
  /**
   * isPassword uses a PasswordField component when `true`; otherwise a
   * TextField component is used.
   */
  isPassword?: boolean
  /**
   * displayValueOverride allows displaying a different value in the field
   * without changing the actual underlying value.
   */
  displayValueOverride?: string

  variant?: "outlined" | "filled" | "standard"
}

/**
 * Factory function for creating a formik TextField
 *
 * @example
 * interface FormValues {
 *   username: string
 * }
 *
 * // Use the factory to create a FormTextField associated to this form
 * const FormTextField = formTextFieldFactory<FormValues>()
 *
 * const MyComponent: React.FC = () => {
 *   const form = useFormik<FormValues>()
 *
 *   return (
 *     <FormTextField
 *       form={form}
 *       formFieldName="username"
 *       fullWidth
 *       helperText="A unique name"
 *       label="Username"
 *       placeholder="Lorem Ipsum"
 *       required
 *     />
 *   )
 * }
 */
export const FormTextField = <T,>({
  children,
  disabled,
  displayValueOverride,
  eventTransform,
  form,
  formFieldName,
  helperText,
  isPassword = false,
  InputProps,
  onChange,
  type,
  variant = "outlined",
  ...rest
}: FormTextFieldProps<T>): ReactElement => {
  const isError = form.touched[formFieldName] && Boolean(form.errors[formFieldName])

  // Conversion to a string primitive is necessary as formFieldName is an in
  // indexable type such as a string, number or enum.
  const fieldId = String(formFieldName)

  const Component = isPassword ? PasswordField : TextField
  const inputType = isPassword ? undefined : type

  return (
    <Component
      {...rest}
      variant={variant}
      disabled={disabled || form.isSubmitting}
      error={isError}
      helperText={isError ? form.errors[formFieldName]?.toString() : helperText}
      id={fieldId}
      InputProps={isPassword ? undefined : InputProps}
      name={fieldId}
      onBlur={form.handleBlur}
      onChange={(e) => {
        if (typeof onChange !== "undefined") {
          onChange(e)
        }

        const event = e
        if (typeof eventTransform !== "undefined" && typeof event.target.value === "string") {
          event.target.value = eventTransform(e.target.value)
        }
        form.handleChange(event)
      }}
      type={inputType}
      value={displayValueOverride || form.values[formFieldName]}
    >
      {children}
    </Component>
  )
}
