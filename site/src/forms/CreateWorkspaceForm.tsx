import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import * as TypesGen from "../api/typesGenerated"
import { FormCloseButton } from "../components/FormCloseButton/FormCloseButton"
import { FormSection } from "../components/FormSection/FormSection"
import { FormTextField } from "../components/FormTextField/FormTextField"
import { FormTitle } from "../components/FormTitle/FormTitle"
import { LoadingButton } from "../components/LoadingButton/LoadingButton"
import { maxWidth } from "../theme/constants"

export interface CreateWorkspaceForm {
  template: TypesGen.Template
  onSubmit: (organizationId: string, request: TypesGen.CreateWorkspaceRequest) => Promise<TypesGen.Workspace>
  onCancel: () => void
  organizationId: string
}

const validationSchema = Yup.object({
  name: Yup.string().required("Name is required"),
})

export const CreateWorkspaceForm: React.FC<CreateWorkspaceForm> = ({
  template,
  onSubmit,
  onCancel,
  organizationId,
}) => {
  const styles = useStyles()

  const form: FormikContextType<{ name: string }> = useFormik<{ name: string }>({
    initialValues: {
      name: "",
    },
    enableReinitialize: true,
    validationSchema: validationSchema,
    onSubmit: ({ name }) => {
      return onSubmit(organizationId, {
        template_id: template.id,
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
            for template <strong>{template.name}</strong>
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
