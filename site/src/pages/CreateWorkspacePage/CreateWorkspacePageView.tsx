import TextField from "@mui/material/TextField"
import * as TypesGen from "api/typesGenerated"
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete"
import { FormikContextType, useFormik } from "formik"
import { FC, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "utils/formUtils"
import * as Yup from "yup"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { SelectedTemplate } from "./SelectedTemplate"
import {
  FormFields,
  FormSection,
  FormFooter,
  HorizontalForm,
} from "components/Form/Form"
import { makeStyles } from "@mui/styles"
import {
  getInitialRichParameterValues,
  useValidationSchemaForRichParameters,
} from "utils/richParameters"
import {
  ImmutableTemplateParametersSection,
  MutableTemplateParametersSection,
} from "components/TemplateParameters/TemplateParameters"
import { CreateWSPermissions } from "xServices/createWorkspace/createWorkspaceXService"
import { GitAuth } from "components/GitAuth/GitAuth"
import { ErrorAlert } from "components/Alert/ErrorAlert"

export interface CreateWorkspacePageViewProps {
  error: unknown
  defaultName: string
  defaultOwner: TypesGen.User
  template: TypesGen.Template
  gitAuth: TypesGen.TemplateVersionGitAuth[]
  parameters: TypesGen.TemplateVersionParameter[]
  defaultBuildParameters: TypesGen.WorkspaceBuildParameter[]
  permissions: CreateWSPermissions
  creatingWorkspace: boolean
  onCancel: () => void
  onSubmit: (req: TypesGen.CreateWorkspaceRequest, owner: TypesGen.User) => void
}

export const CreateWorkspacePageView: FC<CreateWorkspacePageViewProps> = ({
  error,
  defaultName,
  defaultOwner,
  template,
  gitAuth,
  parameters,
  defaultBuildParameters,
  permissions,
  creatingWorkspace,
  onSubmit,
  onCancel,
}) => {
  const { t } = useTranslation("createWorkspacePage")
  const styles = useStyles()
  const [owner, setOwner] = useState(defaultOwner)
  const { verifyGitAuth, gitAuthErrors } = useGitAuthVerification(gitAuth)
  const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
    useFormik<TypesGen.CreateWorkspaceRequest>({
      initialValues: {
        name: defaultName,
        template_id: template.id,
        rich_parameter_values: getInitialRichParameterValues(
          parameters,
          defaultBuildParameters,
        ),
      },
      validationSchema: Yup.object({
        name: nameValidator(t("nameLabel", { ns: "createWorkspacePage" })),
        rich_parameter_values: useValidationSchemaForRichParameters(
          "createWorkspacePage",
          parameters,
        ),
      }),
      enableReinitialize: true,
      onSubmit: (request) => {
        if (!verifyGitAuth()) {
          form.setSubmitting(false)
          return
        }

        onSubmit(request, owner)
      },
    })

  useEffect(() => {
    if (error) {
      window.scrollTo(0, 0)
    }
  }, [error])

  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceRequest>(
    form,
    error,
  )

  return (
    <FullPageHorizontalForm title="New workspace" onCancel={onCancel}>
      <HorizontalForm onSubmit={form.handleSubmit}>
        {Boolean(error) && <ErrorAlert error={error} />}
        {/* General info */}
        <FormSection
          title="General"
          description="The template and name of your new workspace."
        >
          <FormFields>
            <SelectedTemplate template={template} />
            <TextField
              {...getFieldHelpers("name")}
              disabled={form.isSubmitting}
              onChange={onChangeTrimmed(form)}
              autoFocus
              fullWidth
              label={t("nameLabel")}
            />
          </FormFields>
        </FormSection>

        {permissions.createWorkspaceForUser && (
          <FormSection
            title="Workspace Owner"
            description="Only admins can create workspace for other users."
          >
            <FormFields>
              <UserAutocomplete
                value={owner}
                onChange={(user) => {
                  setOwner(user ?? defaultOwner)
                }}
                label={t("ownerLabel").toString()}
                size="medium"
              />
            </FormFields>
          </FormSection>
        )}

        {gitAuth && gitAuth.length > 0 && (
          <FormSection
            title="Git Authentication"
            description="This template requires authentication to automatically perform Git operations on create."
          >
            <FormFields>
              {gitAuth.map((auth, index) => (
                <GitAuth
                  key={index}
                  authenticateURL={auth.authenticate_url}
                  authenticated={auth.authenticated}
                  type={auth.type}
                  error={gitAuthErrors[auth.id]}
                />
              ))}
            </FormFields>
          </FormSection>
        )}

        {parameters && (
          <>
            <MutableTemplateParametersSection
              templateParameters={parameters}
              getInputProps={(parameter, index) => {
                return {
                  ...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  ),
                  onChange: async (value) => {
                    await form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    })
                  },
                  disabled: form.isSubmitting,
                }
              }}
            />
            <ImmutableTemplateParametersSection
              templateParameters={parameters}
              classes={{ root: styles.warningSection }}
              getInputProps={(parameter, index) => {
                return {
                  ...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  ),
                  onChange: async (value) => {
                    await form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    })
                  },
                  disabled: form.isSubmitting,
                }
              }}
            />
          </>
        )}

        <FormFooter
          onCancel={onCancel}
          isLoading={creatingWorkspace}
          submitLabel={t("createWorkspace").toString()}
        />
      </HorizontalForm>
    </FullPageHorizontalForm>
  )
}

type GitAuthErrors = Record<string, string>

const useGitAuthVerification = (gitAuth: TypesGen.TemplateVersionGitAuth[]) => {
  const [gitAuthErrors, setGitAuthErrors] = useState<GitAuthErrors>({})

  useEffect(() => {
    // templateGitAuth is refreshed automatically using a BroadcastChannel
    // which may change the `authenticated` property.
    //
    // If the provider becomes authenticated, we want the error message
    // to disappear.
    setGitAuthErrors({})
  }, [gitAuth])

  const verifyGitAuth = () => {
    const errors: GitAuthErrors = {}

    for (let i = 0; i < gitAuth.length; i++) {
      const auth = gitAuth.at(i)
      if (!auth) {
        continue
      }
      if (!auth.authenticated) {
        errors[auth.id] = "You must authenticate to create a workspace!"
      }
    }

    setGitAuthErrors(errors)
    const isValid = Object.keys(errors).length === 0
    return isValid
  }

  return {
    gitAuthErrors,
    verifyGitAuth,
  }
}

const useStyles = makeStyles((theme) => ({
  warningText: {
    color: theme.palette.warning.light,
  },
  warningSection: {
    border: `1px solid ${theme.palette.warning.light}`,
    borderRadius: 8,
    backgroundColor: theme.palette.background.paper,
    padding: theme.spacing(10),
    marginLeft: theme.spacing(-10),
    marginRight: theme.spacing(-10),
  },
}))
