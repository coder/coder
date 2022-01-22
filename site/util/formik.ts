import { FormikContextType, FormikErrors, FormikTouched, getIn } from "formik"

/**
 * FormikLike is a thin layer of abstraction over 'Formik'
 *
 * FormikLike is intended to be compatible with Formik (ie, a subset - a drop-in replacement),
 * but adds faculty for handling sub-forms.
 */
export interface FormikLike<T>
  extends Pick<
    FormikContextType<T>,
    // Subset of formik functionality that is supported in subForms
    | "errors"
    | "handleBlur"
    | "handleChange"
    | "isSubmitting"
    | "setFieldTouched"
    | "setFieldValue"
    | "submitCount"
    | "touched"
    | "values"
  > {
  getFieldId?: (fieldName: string) => string
}

// Utility functions around the FormikLike interface
export namespace FormikLike {
  /**
   * getFieldId
   *
   * getFieldId returns the fully-qualified path for a field.
   * For a form with no parents, this is just the field name.
   * For a form with parents, this is included the path of the field.
   */
  export const getFieldId = <T>(form: FormikLike<T>, fieldName: string | keyof T): string => {
    if (typeof form.getFieldId !== "undefined") {
      return form.getFieldId(String(fieldName))
    } else {
      return String(fieldName)
    }
  }
}

/**
 * subForm
 *
 * `subForm` takes a parentForm and a selector, and returns a new form
 * that is scoped to just the selector.
 *
 * For example, consider the schema:
 *
 * ```
 * type NestedSchema = {
 *   name: string,
 * }
 *
 * type Schema = {
 *   nestedForm: NestedSchema,
 * }
 * ```
 *
 * Calling `subForm(parentForm, "nestedForm")` where `parentForm` is a
 * `FormikLike<Schema>` will return a `FormikLike<NestedSchema>`.
 *
 * This is helpful for composing forms - a `FormikLike<NestedSchema>`
 * could either be part of a larger parent form, or a stand-alone form -
 * the component itself doesn't have to know!
 *
 * @param parentForm The parent form `FormikLike`
 * @param subFormSelector The field containing the nested form
 * @returns A `FormikLike` for the nested form
 */
export const subForm = <TParentFormSchema, TSubFormSchema>(
  parentForm: FormikLike<TParentFormSchema>,
  subFormSelector: string & keyof TParentFormSchema,
): FormikLike<TSubFormSchema> => {
  // TODO: It would be nice to have better typing for `getIn` so that we
  // don't need the `as` cast. Perhaps future versions of `Formik` will have
  // a more strongly typed version of `getIn`? Or, we may have a more type-safe
  // utility for this in the future.
  const values = getIn(parentForm.values, subFormSelector) as TSubFormSchema
  const errors = (getIn(parentForm.errors, subFormSelector) || {}) as FormikErrors<TSubFormSchema>
  const touched = (getIn(parentForm.touched, subFormSelector) || {}) as FormikTouched<TSubFormSchema>

  const getFieldId = (fieldName: string): string => {
    return FormikLike.getFieldId(parentForm, subFormSelector + "." + fieldName)
  }

  return {
    values,
    errors,
    touched,

    // We can pass the parentForm handlerBlur/handleChange directly,
    // since they figure out the field ID from the element.
    handleBlur: parentForm.handleBlur,
    handleChange: parentForm.handleChange,

    // isSubmitting can just pass through - there isn't a difference
    // in submitting state between parent forms and subforms
    // (only the top-level form handles submission)
    isSubmitting: parentForm.isSubmitting,
    submitCount: parentForm.submitCount,

    // Wrap setFieldValue & setFieldTouched so we can resolve to the fully-nested ID for setting
    setFieldValue: <T extends keyof TSubFormSchema>(fieldName: string | keyof T, value: T[keyof T]): void => {
      const fieldNameAsString = String(fieldName)
      const resolvedFieldId = getFieldId(fieldNameAsString)

      parentForm.setFieldValue(resolvedFieldId, value)
    },
    setFieldTouched: <T extends keyof TSubFormSchema>(
      fieldName: string | keyof T,
      isTouched: boolean | undefined,
      shouldValidate = false,
    ): void => {
      const fieldNameAsString = String(fieldName)
      const resolvedFieldId = getFieldId(fieldNameAsString)

      parentForm.setFieldTouched(resolvedFieldId, isTouched, shouldValidate)
    },

    getFieldId,
  }
}
