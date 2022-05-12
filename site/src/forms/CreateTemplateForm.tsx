import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import * as TypesGen from "../api/typesGenerated"
import { FormCloseButton } from "../components/FormCloseButton/FormCloseButton"
import { FormDropdownField, FormDropdownItem } from "../components/FormDropdownField/FormDropdownField"
import { FormSection } from "../components/FormSection/FormSection"
import { FormTextField } from "../components/FormTextField/FormTextField"
import { FormTitle } from "../components/FormTitle/FormTitle"
import { LoadingButton } from "../components/LoadingButton/LoadingButton"
import { maxWidth } from "../theme/constants"

// It appears that to create a template you need to create a template version
// and then a template so this contains the information to do both.
export type CreateTemplateRequest = TypesGen.CreateTemplateVersionRequest & Pick<TypesGen.CreateTemplateRequest, "name">

export interface CreateTemplateFormProps {
  provisioners: TypesGen.ProvisionerDaemon[]
  organizations: TypesGen.Organization[]
  onSubmit: (organizationId: string, request: CreateTemplateRequest) => Promise<TypesGen.Template>
  onCancel: () => void
}

interface CreateTemplateFields extends Pick<CreateTemplateRequest, "name" | "provisioner"> {
  organizationId: string
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

  const form: FormikContextType<CreateTemplateFields> = useFormik<CreateTemplateFields>({
    initialValues: {
      provisioner: provisioners[0].id,
      organizationId: organizations[0].name,
      name: "",
    },
    enableReinitialize: true,
    validationSchema: validationSchema,
    onSubmit: (req) => {
      return onSubmit(req.organizationId, {
        name: req.name,
        storage_method: "file",
        storage_source: "hash",
        provisioner: req.provisioner,
      })
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
