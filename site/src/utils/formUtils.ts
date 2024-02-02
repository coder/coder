import { isApiValidationError, mapApiErrorToFieldErrors } from "api/errors";
import { FormikContextType, FormikErrors, getIn } from "formik";
import {
  ChangeEvent,
  ChangeEventHandler,
  FocusEventHandler,
  ReactNode,
} from "react";
import * as Yup from "yup";

const Language = {
  nameRequired: (name: string): string => {
    return name ? `Please enter a ${name.toLowerCase()}.` : "Required";
  },
  nameInvalidChars: (name: string): string => {
    return `${name} must start with a-Z or 0-9 and can contain a-Z, 0-9 or -`;
  },
  nameTooLong: (name: string, len: number): string => {
    return `${name} cannot be longer than ${len} characters`;
  },
  templateDisplayNameInvalidChars: (name: string): string => {
    return `${name} must start and end with non-whitespace character`;
  },
};

interface GetFormHelperOptions {
  helperText?: ReactNode;
  /**
   * backendFieldName remaps the name in the form, for when it doesn't match the
   * name used by the backend
   */
  backendFieldName?: string;
  /**
   * maxLength is used for showing helper text on fields that have a limited length,
   * which will let the user know how much space they have left, or how much they are
   * over the limit. Zero and negative values will be ignored.
   */
  maxLength?: number;
}

interface FormHelpers {
  name: string;
  onBlur: FocusEventHandler<HTMLInputElement | HTMLTextAreaElement>;
  onChange: ChangeEventHandler<HTMLInputElement | HTMLTextAreaElement>;
  id: string;
  value?: string | number;
  error: boolean;
  helperText?: ReactNode;
}

export const getFormHelpers =
  <TFormValues>(form: FormikContextType<TFormValues>, error?: unknown) =>
  (
    fieldName: keyof TFormValues | string,
    options: GetFormHelperOptions = {},
  ): FormHelpers => {
    const {
      backendFieldName,
      helperText: defaultHelperText,
      maxLength,
    } = options;
    let helperText = defaultHelperText;
    const apiValidationErrors = isApiValidationError(error)
      ? (mapApiErrorToFieldErrors(
          error.response.data,
        ) as FormikErrors<TFormValues> & { [key: string]: string })
      : undefined;
    // Since the fieldName can be a path string like parameters[0].value we need to use getIn
    const touched = Boolean(getIn(form.touched, fieldName.toString()));
    const formError = getIn(form.errors, fieldName.toString());
    // Since the field in the form can be different from the backend, we need to
    // check for both when getting the error
    const apiField = backendFieldName ?? fieldName;
    const apiError = apiValidationErrors?.[apiField.toString()];

    const fieldProps = form.getFieldProps(fieldName);
    const value = fieldProps.value;

    let lengthError: ReactNode = null;
    // Show a message if the input is approaching or over the maximum length.
    if (
      maxLength &&
      maxLength > 0 &&
      typeof value === "string" &&
      value.length > maxLength - 30
    ) {
      helperText = `This cannot be longer than ${maxLength} characters. (${value.length}/${maxLength})`;
      // Show it as an error, rather than a hint
      if (value.length > maxLength) {
        lengthError = helperText;
      }
    }

    // API and regular validation errors should wait to be shown, but length errors should
    // be more responsive.
    const errorToDisplay =
      (touched && apiError) || lengthError || (touched && formError);

    return {
      ...fieldProps,
      id: fieldName.toString(),
      error: Boolean(errorToDisplay),
      helperText: errorToDisplay || helperText,
    };
  };

export const onChangeTrimmed =
  <T>(form: FormikContextType<T>, callback?: (value: string) => void) =>
  (event: ChangeEvent<HTMLInputElement>): void => {
    event.target.value = event.target.value.trim();
    form.handleChange(event);
    callback?.(event.target.value);
  };

// REMARK: Keep these consts in sync with coderd/httpapi/httpapi.go
const maxLenName = 32;
const templateDisplayNameMaxLength = 64;
const usernameRE = /^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$/;
const templateDisplayNameRE = /^[^\s](.*[^\s])?$/;

// REMARK: see #1756 for name/username semantics
export const nameValidator = (name: string): Yup.StringSchema =>
  Yup.string()
    .required(Language.nameRequired(name))
    .matches(usernameRE, Language.nameInvalidChars(name))
    .max(maxLenName, Language.nameTooLong(name, maxLenName));

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
    .optional();

export const iconValidator = Yup.string().label("Icon").max(256);
