import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { CreateTemplateRequest, Organization, Provisioner, Template } from "../api/types"
import { FormCloseButton } from "../components/FormCloseButton/FormCloseButton"
import { FormDropdownField, FormDropdownItem } from "../components/FormDropdownField/FormDropdownField"
import { FormSection } from "../components/FormSection/FormSection"
import { FormTextField } from "../components/FormTextField/FormTextField"
import { FormTitle } from "../components/FormTitle/FormTitle"
import { LoadingButton } from "../components/LoadingButton/LoadingButton"
import { maxWidth } from "../theme/constants"

export interface CreateTemplateFormProps {
  provisioners: Provisioner[]
  organizations: Organization[]
  onSubmit: (request: CreateTemplateRequest) => Promise<Template>
  onCancel: () => void
}

const validationSchema = Yup.object({
  provisioner: Yup.string().required("Provisioner is required."),
  organizationId: Yup.string().required("Organization is required."),
  name: Yup.string().required("Name is required"),
})

export const CreateTemplateForm: React.FC<CreateTemplateFormProps> = ({
  provisioners,
  organizations,
  onSubmit,
  onCancel,
}) => {
  const styles = useStyles()

  const form: FormikContextType<CreateTemplateRequest> = useFormik<CreateTemplateRequest>({
    initialValues: {
      provisioner: provisioners[0].id,
      organizationId: organizations[0].name,
      name: "",
    },
    enableReinitialize: true,
    validationSchema: validationSchema,
    onSubmit: (req) => {
      return onSubmit(req)
    },
  })

  const organizationDropDownItems: FormDropdownItem[] = organizations.map((org) => {
    return {
      value: org.name,
      name: org.name,
    }
  })

  const provisionerDropDownItems: FormDropdownItem[] = provisioners.map((provisioner) => {
    return {
      value: provisioner.id,
      name: provisioner.name,
    }
  })

  return (
    <div className={styles.root}>
      <FormTitle title="Create Template" />
      <FormCloseButton onClose={onCancel} />

      <FormSection title="Name">
        <FormTextField
          form={form}
          formFieldName="name"
          fullWidth
          helperText="A unique name describing your template."
          label="Template Name"
          placeholder="my-template"
          required
        />
      </FormSection>

      <FormSection title="Organization">
        <FormDropdownField
          form={form}
          formFieldName="organizationId"
          helperText="The organization owning this template."
          items={organizationDropDownItems}
          fullWidth
          select
          required
        />
      </FormSection>

      <FormSection title="Provider">
        <FormDropdownField
          form={form}
          formFieldName="provisioner"
          helperText="The backing provisioner for this template."
          items={provisionerDropDownItems}
          fullWidth
          select
          required
        />
      </FormSection>

      <div className={styles.footer}>
        <Button className={styles.button} onClick={onCancel} variant="outlined">
          Cancel
        </Button>
        <LoadingButton
          loading={form.isSubmitting}
          className={styles.button}
          onClick={form.submitForm}
          variant="contained"
          color="primary"
          type="submit"
        >
          Submit
        </LoadingButton>
      </div>
    </div>
  )
}

const useStyles = makeStyles(() => ({
  root: {
    maxWidth,
    width: "100%",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
  },
  footer: {
    display: "flex",
    flex: "0",
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
  },
  button: {
    margin: "1em",
  },
}))
