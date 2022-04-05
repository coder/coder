import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { CreateWorkspaceRequest, Project, Workspace } from "../api/types"
import { LoadingButton } from "../components/Button"
import { FormCloseButton, FormSection, FormTextField, FormTitle } from "../components/Form"

export interface CreateWorkspaceForm {
  project: Project
  onSubmit: (request: CreateWorkspaceRequest) => Promise<Workspace>
  onCancel: () => void
}

const validationSchema = Yup.object({
  name: Yup.string().required("Name is required"),
})

export const CreateWorkspaceForm: React.FC<CreateWorkspaceForm> = ({ project, onSubmit, onCancel }) => {
  const styles = useStyles()

  const form: FormikContextType<{ name: string }> = useFormik<{ name: string }>({
    initialValues: {
      name: "",
    },
    enableReinitialize: true,
    validationSchema: validationSchema,
    onSubmit: ({ name }) => {
      return onSubmit({
        project_id: project.id,
        name: name,
      })
    },
  })

  return (
    <div className={styles.root}>
      <FormTitle
        title="Create Workspace"
        detail={
          <span>
            for project <strong>{project.name}</strong>
          </span>
        }
      />
      <FormCloseButton onClose={onCancel} />

      <FormSection title="Name">
        <FormTextField
          form={form}
          formFieldName="name"
          fullWidth
          helperText="A unique name describing your workspace."
          label="Workspace Name"
          placeholder="my-workspace"
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
    maxWidth: "1380px",
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
