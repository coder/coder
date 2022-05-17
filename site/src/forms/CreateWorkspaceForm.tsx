import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import * as TypesGen from "../api/typesGenerated"
import { FormCloseButton } from "../components/FormCloseButton/FormCloseButton"
import { FormSection } from "../components/FormSection/FormSection"
import { FormTitle } from "../components/FormTitle/FormTitle"
import { LoadingButton } from "../components/LoadingButton/LoadingButton"
import { maxWidth } from "../theme/constants"
import { getFormHelpers, onChangeTrimmed } from "../util/formUtils"

export const Language = {
  nameHelperText: "A unique name describing your workspace",
  nameLabel: "Workspace Name",
  nameMatches: "Name must start with a-Z or 0-9 and can contain a-Z, 0-9 or -",
  nameMax: "Name cannot be longer than 32 characters",
  namePlaceholder: "my-workspace",
  nameRequired: "Name is required",
}
export interface CreateWorkspaceForm {
  template: TypesGen.Template
  onSubmit: (organizationId: string, request: TypesGen.CreateWorkspaceRequest) => Promise<TypesGen.Workspace>
  onCancel: () => void
  organizationId: string
}

export interface CreateWorkspaceFormValues {
  name: string
}

// REMARK: Keep in sync with coderd/httpapi/httpapi.go#L40
const maxLenName = 32

// REMARK: Keep in sync with coderd/httpapi/httpapi.go#L18
const usernameRE = /^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$/

export const validationSchema = Yup.object({
  name: Yup.string()
    .matches(usernameRE, Language.nameMatches)
    .max(maxLenName, Language.nameMax)
    .required(Language.nameRequired),
})

export const CreateWorkspaceForm: React.FC<CreateWorkspaceForm> = ({
  template,
  onSubmit,
  onCancel,
  organizationId,
}) => {
  const styles = useStyles()

  const form = useFormik<CreateWorkspaceFormValues>({
    initialValues: {
      name: "",
    },
    onSubmit: ({ name }) => {
      return onSubmit(organizationId, {
        template_id: template.id,
        name: name,
      })
    },
    validationSchema: validationSchema,
  })
  const getFieldHelpers = getFormHelpers<CreateWorkspaceFormValues>(form)

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
        <TextField
          {...getFieldHelpers("name", Language.nameHelperText)}
          onChange={onChangeTrimmed(form)}
          autoFocus
          fullWidth
          label={Language.nameLabel}
          placeholder={Language.namePlaceholder}
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
