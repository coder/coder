import { act, fireEvent, render, screen } from "@testing-library/react"
import { useFormik } from "formik"
import { FC } from "react"
import * as yup from "yup"
import { FormTextField, FormTextFieldProps } from "./FormTextField"

namespace Helpers {
  export interface FormValues {
    name: string
  }

  export const requiredValidationMsg = "required"

  export const Component: FC<React.PropsWithChildren<Omit<FormTextFieldProps<FormValues>, "form" | "formFieldName">>> = (
    props,
  ) => {
    const form = useFormik<FormValues>({
      initialValues: {
        name: "",
      },
      onSubmit: (values, helpers) => {
        return helpers.setSubmitting(false)
      },
      validationSchema: yup.object({
        name: yup.string().required(requiredValidationMsg),
      }),
    })

    return <FormTextField<FormValues> {...props} form={form} formFieldName="name" />
  }
}

describe("FormTextField", () => {
  describe("helperText", () => {
    it("uses helperText prop when there are no errors", () => {
      // Given
      const props = {
        helperText: "testing",
      }

      // When
      const { queryByText } = render(<Helpers.Component {...props} />)

      // Then
      expect(queryByText(props.helperText)).toBeDefined()
    })

    it("uses validation message when there are errors", () => {
      // Given
      const props = {}

      // When
      const { container } = render(<Helpers.Component {...props} />)
      const el = container.firstChild

      // Then
      expect(el).toBeDefined()
      expect(screen.queryByText(Helpers.requiredValidationMsg)).toBeNull()

      // When
      act(() => {
        fireEvent.focus(el as Element)
      })

      // Then
      expect(screen.queryByText(Helpers.requiredValidationMsg)).toBeNull()

      // When
      act(() => {
        fireEvent.blur(el as Element)
      })

      // Then
      expect(screen.queryByText(Helpers.requiredValidationMsg)).toBeDefined()
    })
  })
})
