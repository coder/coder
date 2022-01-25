import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { useFormik } from "formik"
import React from "react"
import * as Yup from "yup"

import {
  FormTitle,
  FormSection,
  formTextFieldFactory,
  formDropdownFieldFactory,
  DropdownItem,
} from "../components/Form"

import { Organization, Provisioner } from "./../api"

export interface CreateProjectRequest {
  provisionerId: string
  organizationId: string
  name: string
}

export interface CreateProjectFormProps {
  provisioners: Provisioner[]
  organizations: Organization[]
  onSubmit: (request: CreateProjectRequest) => Promise<void>
  onCancel: () => void
}

const validationSchema = Yup.object({
  provisionerId: Yup.string().required("Email is required."),
  organizationId: Yup.string().required("Organization is required."),
  name: Yup.string().required("Name is required"),
})

const FormTextField = formTextFieldFactory<CreateProjectRequest>()
const FormDropdownField = formDropdownFieldFactory<CreateProjectRequest>()

export const CreateProjectForm: React.FC<CreateProjectFormProps> = ({
  provisioners,
  organizations,
  onSubmit,
  onCancel,
}) => {
  const styles = useStyles()

  const form = useFormik<CreateProjectRequest>({
    initialValues: {
      provisionerId: provisioners[0].id,
      organizationId: organizations[0].id,
      name: "",
    },
    enableReinitialize: true,
    validationSchema: validationSchema,
    onSubmit: onSubmit,
  })

  const organizationDropDownItems: DropdownItem[] = organizations.map((org) => {
    return {
      value: org.id,
      name: org.name,
    }
  })

  const provisionerDropDownItems: DropdownItem[] = provisioners.map((provisioner) => {
    return {
      value: provisioner.id,
      name: provisioner.name,
    }
  })

  return (
    <div className={styles.root}>
      <FormTitle title="Create Project" />

      <FormSection title="Name">
        <FormTextField
          form={form}
          formFieldName="name"
          fullWidth
          helperText="A unique name describing your project."
          label="Project Name"
          placeholder={"my-project"}
          required
        />
      </FormSection>

      <FormSection title="Organization">
        <FormDropdownField
          form={form}
          formFieldName={"organizationId"}
          helperText="The organization owning this project."
          items={organizationDropDownItems}
          fullWidth
          select
          required
        />
      </FormSection>

      <FormSection title="Provider">
        <FormDropdownField
          form={form}
          formFieldName={"provisionerId"}
          helperText="The backing provisioner for this project."
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
        <Button className={styles.button} onClick={form.submitForm} variant="contained" color="primary" type="submit">
          Submit
        </Button>
      </div>
    </div>
  )
}

const useStyles = makeStyles(() => ({
  root: {
    maxWidth: "1380px",
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
