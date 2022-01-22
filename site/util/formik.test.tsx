import Checkbox from "@material-ui/core/Checkbox"
import "@testing-library/jest-dom"
import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useFormik } from "formik"
import { formTextFieldFactory } from "../components/Form"
import React from "react"
import * as Yup from "yup"
import { FormikLike, subForm } from "./formik"

/**
 * SubForm
 *
 * A simple schema component for a small 'subform' that we'll reuse
 * across a bunch of different forms.
 *
 * With the `subForm` API, this can act as either a top-level form,
 * or can be composed as sub-forms within other forms.
 */
namespace SubForm {
  export type Schema = {
    firstName?: string
    lastName?: string
  }

  export const validator = Yup.object({
    firstName: Yup.string().required().min(3, "First name must be at least characters"),
    lastName: Yup.string().required(),
  })

  export interface SubFormProps {
    form: FormikLike<Schema>
  }

  const FormTextField = formTextFieldFactory<Schema>()

  export const Component: React.FC<SubFormProps> = (props: SubFormProps) => {
    const { form } = props
    return (
      <>
        <div className="firstName-container">
          <FormTextField form={form} formFieldName="firstName" helperText="Your first name" label="First Name" />
        </div>
        <div className="lastName-container">
          <FormTextField form={form} formFieldName="lastName" helperText="Your last name" label="Last Name" />
        </div>
      </>
    )
  }
}

namespace SubFormUsingSetValue {
  export type Schema = {
    count: number
  }

  export const initialValues = {
    count: 0,
  }

  export const validator = Yup.object({
    count: Yup.number(),
  })

  export interface SubFormUsingSetValueProps {
    form: FormikLike<Schema>
  }

  export const Component: React.FC<SubFormUsingSetValueProps> = (props: SubFormUsingSetValueProps) => {
    const { form } = props
    const currentValue = form.values.count

    const incrementCount = () => {
      form.setFieldValue("count", currentValue + 1)
    }

    return (
      <>
        <div>{"Count: " + currentValue.toString()}</div>
        <input type="button" value="Click me to increment count" onClick={incrementCount} />
      </>
    )
  }
}

/**
 * FormWithNestedSubForm
 *
 * This is an example of a form that nests the SubForm.
 */
namespace FormWithNestedSubForm {
  export type Schema = {
    parentField: string
    subForm: SubForm.Schema
  }

  export const validator = Yup.object({
    parentField: Yup.string().required(),
    subForm: SubForm.validator,
  })

  export interface FormWithNestedSubFormProps {
    form: FormikLike<Schema>
  }

  export const Component: React.FC<FormWithNestedSubFormProps> = (props: FormWithNestedSubFormProps) => {
    const { form } = props

    const nestedForm = subForm<Schema, SubForm.Schema>(form, "subForm")
    return (
      <div>
        <div id="parentForm"></div>
        <div id="subForm">
          <SubForm.Component form={nestedForm} />
        </div>
      </div>
    )
  }
}

/**
 * FormWithMultipleSubforms
 *
 * Example of a parent form that has multiple child subforms at the same level.
 */
namespace FormWithMultipleNestedSubForms {
  export type Schema = {
    parentField: string
    subForm1: SubForm.Schema
    subForm2: SubForm.Schema
  }

  export const schema = Yup.object({
    parentField: Yup.string().required(),
    subForm1: SubForm.validator,
    subForm2: SubForm.validator,
  })

  export interface FormWithMultipleNestedSubFormProps {
    form: FormikLike<Schema>
  }

  export const Component: React.FC<FormWithMultipleNestedSubFormProps> = (
    props: FormWithMultipleNestedSubFormProps,
  ) => {
    const { form } = props

    const nestedForm1 = subForm<Schema, SubForm.Schema>(form, "subForm1")
    const nestedForm2 = subForm<Schema, SubForm.Schema>(form, "subForm2")
    return (
      <div>
        <div id="parentForm"></div>
        <div id="subForm1">
          <SubForm.Component form={nestedForm1} />
        </div>
        <div id="subForm2">
          <SubForm.Component form={nestedForm2} />
        </div>
      </div>
    )
  }
}

/**
 * FormWithDynamicSubForms
 *
 * This is intended to closely replicate the scenario we'll need for EC2 providers -
 * a dynamic create-workspace form that will show an advanced section that depends on
 * whether the provider is Kubernetes or EC2.
 *
 * This is one approach to designing a form like this, using a 'fat interface'.
 *
 * In this, the schema contains both EC2 | Kubernetes, and the validation
 * logic switches depending on which is chosen.
 */
namespace FormWithDynamicSubForms {
  export type Schema = {
    isKubernetes: boolean
    kubernetesMetadata: SubForm.Schema
    ec2Metadata: SubForm.Schema
  }

  export const schema = Yup.object({
    isKubernetes: Yup.boolean().required(),
    kubernetesMetadata: Yup.mixed().when("isKubernetes", {
      is: true,
      then: SubForm.validator,
    }),
    ec2Metadata: Yup.mixed().when("isKubernetes", {
      is: false,
      then: SubForm.validator,
    }),
  })

  export interface FormWithDynamicSubFormsProps {
    form: FormikLike<Schema>
  }

  export const Component: React.FC<FormWithDynamicSubFormsProps> = (props: FormWithDynamicSubFormsProps) => {
    const { form } = props

    const isKubernetes = form.values.isKubernetes

    const kubernetesForm = subForm<Schema, SubForm.Schema>(form, "kubernetesMetadata")
    const ec2Form = subForm<Schema, SubForm.Schema>(form, "ec2Metadata")
    return (
      <div>
        <div id="parentForm">
          <Checkbox id="isKubernetes" name="isKubernetes" checked={isKubernetes} onChange={form.handleChange} />
        </div>
        {isKubernetes ? (
          <div id="kubernetes">
            <SubForm.Component form={kubernetesForm} />
          </div>
        ) : (
          <div id="ec2">
            <SubForm.Component form={ec2Form} />
          </div>
        )}
      </div>
    )
  }
}

describe("formik", () => {
  describe("subforms", () => {
    // This test is a bit superfluous, but it's here to exhibit the difference in using the sub-form
    // as a top-level form, vs some other form's subform
    it("binds fields correctly as a top-level form", () => {
      // Given
      const TestComponent = () => {
        // Initialize form
        const form = useFormik<SubForm.Schema>({
          initialValues: {
            firstName: "first-name",
            lastName: "last-name",
          },
          validationSchema: SubForm.validator,
          onSubmit: () => {
            return
          },
        })

        return (
          <div id="container">
            <SubForm.Component form={form} />
          </div>
        )
      }

      // When
      const rendered = render(<TestComponent />)

      // Then: verify form gets bound correctly
      const firstNameElement = rendered.container.querySelector(".firstName-container input") as HTMLInputElement
      expect(firstNameElement.value).toBe("first-name")

      const lastNameElement = rendered.container.querySelector(".lastName-container input") as HTMLInputElement
      expect(lastNameElement.value).toBe("last-name")
    })

    it("binds fields correctly as a nested form", () => {
      // Given
      const TestComponent = () => {
        const form = useFormik<FormWithNestedSubForm.Schema>({
          initialValues: {
            parentField: "parent-test-value",
            subForm: {
              firstName: "first-name",
              lastName: "last-name",
            },
          },
          validationSchema: FormWithNestedSubForm.validator,
          onSubmit: () => {
            return
          },
        })

        return <FormWithNestedSubForm.Component form={form} />
      }

      // When
      const rendered = render(<TestComponent />)

      // Then: verify form gets bound correctly
      const firstNameElement = rendered.container.querySelector(
        "#subForm .firstName-container input",
      ) as HTMLInputElement
      expect(firstNameElement.value).toBe("first-name")

      const lastNameElement = rendered.container.querySelector("#subForm .lastName-container input") as HTMLInputElement
      expect(lastNameElement.value).toBe("last-name")
    })

    it("binds fields correctly with multiple nested forms", () => {
      // Given
      const TestComponent = () => {
        const form = useFormik<FormWithMultipleNestedSubForms.Schema>({
          initialValues: {
            parentField: "parent-test-value",
            subForm1: {
              firstName: "Arthur",
              lastName: "Aardvark",
            },
            subForm2: {
              firstName: "Bartholomew",
              lastName: "Bear",
            },
          },
          validationSchema: FormWithNestedSubForm.validator,
          onSubmit: () => {
            return
          },
        })

        return <FormWithMultipleNestedSubForms.Component form={form} />
      }

      // When
      const rendered = render(<TestComponent />)

      // Then: verify form gets bound correctly, for the first nested form
      const firstNameElement1 = rendered.container.querySelector(
        "#subForm1 .firstName-container input",
      ) as HTMLInputElement
      expect(firstNameElement1.value).toBe("Arthur")

      const lastNameElement1 = rendered.container.querySelector(
        "#subForm1 .lastName-container input",
      ) as HTMLInputElement
      expect(lastNameElement1.value).toBe("Aardvark")

      // Verify form gets bound correctly, for the first nested form
      const firstNameElement2 = rendered.container.querySelector(
        "#subForm2 .firstName-container input",
      ) as HTMLInputElement
      expect(firstNameElement2.value).toBe("Bartholomew")

      const lastNameElement2 = rendered.container.querySelector(
        "#subForm2 .lastName-container input",
      ) as HTMLInputElement
      expect(lastNameElement2.value).toBe("Bear")
    })

    it("dynamic subforms work correctly", async () => {
      // Given
      const TestComponent = () => {
        const form = useFormik<FormWithDynamicSubForms.Schema>({
          initialValues: {
            isKubernetes: true,
            kubernetesMetadata: {
              firstName: "Kubernetes",
              lastName: "Provider",
            },
            ec2Metadata: {
              firstName: "Amazon",
              lastName: "Provider",
            },
          },
          validationSchema: FormWithDynamicSubForms.schema,
          onSubmit: () => {
            return
          },
        })

        return <FormWithDynamicSubForms.Component form={form} />
      }

      // When
      const rendered = render(<TestComponent />)

      const kubernetesNameElement = rendered.container.querySelector(
        "#kubernetes .firstName-container input",
      ) as HTMLInputElement
      expect(kubernetesNameElement.value).toBe("Kubernetes")

      const checkBox = rendered.container.querySelector("#isKubernetes") as HTMLInputElement

      fireEvent.click(checkBox)
      // Wait for rendering to complete after clicking the checkbox.
      // We know it's done when the 'Amazon' text input displays
      await screen.findByDisplayValue("Amazon")

      // Then
      // Now, we should be in EC2 mode - validate that value got bound correclty
      // We're in kubernetes mode - verify values got bound correctly
      const ec2NameElement = rendered.container.querySelector("#ec2 .firstName-container input") as HTMLInputElement
      expect(ec2NameElement.value).toBe("Amazon")
    })

    it("nested 'touched' gets updated for subform when field is modified", async () => {
      // Given
      const TestComponent = () => {
        // Initialize form
        const form = useFormik<FormWithNestedSubForm.Schema>({
          initialValues: {
            parentField: "no-op",
            subForm: {
              firstName: "",
              lastName: "last-name",
            },
          },
          validationSchema: FormWithNestedSubForm.validator,
          onSubmit: () => {
            return
          },
        })

        const nestedForm = subForm<FormWithNestedSubForm.Schema, SubForm.Schema>(form, "subForm")

        return (
          <div id="container">
            <div className="touched-sensor">{"Touched: " + String(!!nestedForm.touched["firstName"])}</div>
            <SubForm.Component form={nestedForm} />
          </div>
        )
      }

      // When
      const rendered = render(<TestComponent />)
      const touchSensor = rendered.container.querySelector(".touched-sensor") as HTMLDivElement
      expect(touchSensor.textContent).toBe("Touched: false")

      let firstNameElement = rendered.container.querySelector(".firstName-container input") as HTMLInputElement
      // ...user types coder, and leaves form field
      userEvent.type(firstNameElement, "Coder")
      fireEvent.blur(firstNameElement)

      // Then: Verify touched field is updated and everything is up-to-date
      await screen.findByText("Touched: true")

      // Then: verify form gets bound correctly
      firstNameElement = rendered.container.querySelector(".firstName-container input") as HTMLInputElement
      expect(firstNameElement.value).toBe("Coder")
    })

    it.each([
      // The name field is required to be at least 3 characters, so error should be false:
      ["Coder", "error: false"],
      // The name field is required to be at least 3 characters, so error should be true:
      ["C", "error: true"],
    ])("Nested 'error' test - Typing %p should result in %p", async (textToType: string, expectedLabel: string) => {
      // Given
      const TestComponent = () => {
        // Initialize form
        const form = useFormik<FormWithNestedSubForm.Schema>({
          initialValues: {
            parentField: "no-op",
            subForm: {
              // First name needs to be at least 3 characters
              firstName: "",
              lastName: "last-name",
            },
          },
          validationSchema: FormWithNestedSubForm.validator,
          onSubmit: () => {
            return
          },
        })

        const nestedForm = subForm<FormWithNestedSubForm.Schema, SubForm.Schema>(form, "subForm")

        return (
          <div id="container">
            <div className="error-sensor">{"error: " + String(!!nestedForm.errors["firstName"])}</div>
            <SubForm.Component form={nestedForm} />
          </div>
        )
      }

      // When
      const rendered = render(<TestComponent />)

      // Then
      // ...user types coder, and leaves form field
      const firstNameElement = rendered.container.querySelector(".firstName-container input") as HTMLInputElement
      userEvent.type(firstNameElement, textToType)
      fireEvent.blur(firstNameElement)

      const element = await screen.findByText(expectedLabel)
      expect(element.textContent).toBe(expectedLabel)
    })

    it("subforms pass correct values on 'submit'", async () => {
      // Given
      let hitCount = 0
      let submitResult: FormWithNestedSubForm.Schema | null = null

      const onSubmit = (submitValues: FormWithNestedSubForm.Schema) => {
        hitCount++
        submitResult = submitValues
      }

      const TestComponent = () => {
        // Initialize form
        const form = useFormik<FormWithNestedSubForm.Schema>({
          initialValues: {
            parentField: "no-op",
            subForm: {
              // First name needs to be at least 3 characters
              firstName: "",
              lastName: "",
            },
          },
          validationSchema: FormWithNestedSubForm.validator,

          // Submit is always handled by the top-level form
          onSubmit,
        })

        const nestedForm = subForm<FormWithNestedSubForm.Schema, SubForm.Schema>(form, "subForm")

        return (
          <div id="container">
            <div className="submit-sensor">{"Submits: " + String(form.submitCount)}</div>
            <SubForm.Component form={nestedForm} />
            <input type="button" onClick={form.submitForm} value="Submit" />
          </div>
        )
      }

      // When: User types values and submits
      const rendered = render(<TestComponent />)

      const firstNameElement = rendered.container.querySelector(".firstName-container input") as HTMLInputElement
      userEvent.type(firstNameElement, "Coder")
      fireEvent.blur(firstNameElement)

      const lastNameElement = rendered.container.querySelector(".lastName-container input") as HTMLInputElement
      userEvent.type(lastNameElement, "Rocks")
      fireEvent.blur(lastNameElement)

      const submitButton = await screen.findByText("Submit")
      fireEvent.click(submitButton)

      // Wait for submission to percolate through rendering
      await screen.findByText("Submits: 1")

      // Then: We should've received a submit callback with correct values
      expect(hitCount).toBe(1)
      expect(submitResult).toEqual({
        parentField: "no-op",
        subForm: {
          firstName: "Coder",
          lastName: "Rocks",
        },
      })
    })

    it("subforms handle setFieldValue correctly", async () => {
      // Given: A form with a subform that uses `setFieldValue`
      interface ParentSchema {
        subForm: SubFormUsingSetValue.Schema
      }

      const TestComponent = () => {
        // Initialize form
        const form = useFormik<ParentSchema>({
          initialValues: {
            subForm: SubFormUsingSetValue.initialValues,
          },
          validationSchema: Yup.object({
            subForm: SubFormUsingSetValue.validator,
          }),

          // Submit is always handled by the top-level form
          onSubmit: () => {
            return
          },
        })

        const nestedForm = subForm<ParentSchema, SubFormUsingSetValue.Schema>(form, "subForm")

        return (
          <div id="container">
            <SubFormUsingSetValue.Component form={nestedForm} />
          </div>
        )
      }

      render(<TestComponent />)

      // First render: We should find an element with 'Count: 0' (initial value)
      await screen.findAllByText("Count: 0")

      // When: User clicks button, which should increment form value
      const buttonElement = await screen.findByText("Click me to increment count")
      fireEvent.click(buttonElement)

      // Then: The count sensor should be incremented from 0 -> 1
      await screen.findAllByText("Count: 1")
    })

    it("subforms handle setFieldTouched correctly", async () => {
      // Given: A form with a subform that uses `set`
      interface ParentSchema {
        nestedForm: SubForm.Schema
      }

      const TestComponent = () => {
        // Initialize form
        const form = useFormik<ParentSchema>({
          initialValues: {
            nestedForm: {
              firstName: "first",
              lastName: "last",
            },
          },
          validationSchema: Yup.object({
            nestedForm: SubForm.validator,
          }),

          // Submit is always handled by the top-level form
          onSubmit: () => {
            return
          },
        })

        const nestedForm = subForm<ParentSchema, SubForm.Schema>(form, "nestedForm")

        const setTouched = () => {
          nestedForm.setFieldTouched("firstName", true)
        }

        const isFieldTouched = nestedForm.touched["firstName"]
        return (
          <div id="container">
            <div className="touch-sensor">{"Touched: " + String(!!isFieldTouched)}</div>
            <input type="button" onClick={setTouched} value="Click me to set touched" />
            <SubForm.Component form={nestedForm} />
          </div>
        )
      }

      render(<TestComponent />)

      // First render: We should find an element with 'Count: 0' (initial value)
      await screen.findAllByText("Touched: false")

      // When: User clicks button, which should increment form value
      const buttonElement = await screen.findByText("Click me to set touched")
      fireEvent.click(buttonElement)

      // Then: The count sensor should be incremented from 0 -> 1
      await screen.findAllByText("Touched: true")
    })

    it("multiple nesting levels are handled correctly", async () => {
      // Given: A form with a subform that uses `set`
      interface ParentSchema {
        outerNesting: {
          innerNesting: {
            nestedForm: SubForm.Schema
          }
        }
      }

      const TestComponent = () => {
        // Initialize form
        const form = useFormik<ParentSchema>({
          initialValues: {
            outerNesting: {
              innerNesting: {
                nestedForm: {
                  firstName: "double-nested-first",
                  lastName: "",
                },
              },
            },
          },
          validationSchema: Yup.object({
            nestedForm: SubForm.validator,
          }),

          // Submit is always handled by the top-level form
          onSubmit: () => {
            return
          },
        })

        // Peel apart the layers of nesting, so we can validate the binding is correct...
        const outerNestedForm = subForm<ParentSchema, { innerNesting: { nestedForm: SubForm.Schema } }>(
          form,
          "outerNesting",
        )
        const innerNestedForm = subForm<
          { innerNesting: { nestedForm: SubForm.Schema } },
          { nestedForm: SubForm.Schema }
        >(outerNestedForm, "innerNesting")
        const nestedForm = subForm<{ nestedForm: SubForm.Schema }, SubForm.Schema>(innerNestedForm, "nestedForm")

        return (
          <div id="container">
            <div data-testid="field-id-sensor">{"FieldId: " + FormikLike.getFieldId(nestedForm, "firstName")}</div>
            <SubForm.Component form={nestedForm} />
          </div>
        )
      }

      // When
      render(<TestComponent />)

      // Then:
      // The form element should be bound correctly
      const element = await screen.findByTestId("field-id-sensor")
      expect(element.textContent).toBe("FieldId: outerNesting.innerNesting.nestedForm.firstName")
    })
  })
})
