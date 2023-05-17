import TextField from "@mui/material/TextField"
import * as TypesGen from "api/typesGenerated"
import { ParameterInput } from "components/ParameterInput/ParameterInput"
import { Stack } from "components/Stack/Stack"
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "utils/formUtils"
import * as Yup from "yup"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { SelectedTemplate } from "./SelectedTemplate"
import { Loader } from "components/Loader/Loader"
import { GitAuth } from "components/GitAuth/GitAuth"
import {
  FormFields,
  FormSection,
  FormFooter,
  HorizontalForm,
} from "components/Form/Form"
import { makeStyles } from "@mui/styles"
import {
  selectInitialRichParametersValues,
  useValidationSchemaForRichParameters,
  workspaceBuildParameterValue,
} from "utils/richParameters"
import {
  ImmutableTemplateParametersSection,
  MutableTemplateParametersSection,
} from "components/TemplateParameters/TemplateParameters"
import { ErrorAlert } from "components/Alert/ErrorAlert"

export enum CreateWorkspaceErrors {
  GET_TEMPLATES_ERROR = "getTemplatesError",
  GET_TEMPLATE_SCHEMA_ERROR = "getTemplateSchemaError",
  GET_TEMPLATE_GITAUTH_ERROR = "getTemplateGitAuthError",
  CREATE_WORKSPACE_ERROR = "createWorkspaceError",
}

export interface CreateWorkspacePageViewProps {
  name: string
  loadingTemplates: boolean
  loadingTemplateSchema: boolean
  creatingWorkspace: boolean
  hasTemplateErrors: boolean
  templateName: string
  templates?: TypesGen.Template[]
  selectedTemplate?: TypesGen.Template
  templateParameters?: TypesGen.TemplateVersionParameter[]
  templateSchema?: TypesGen.ParameterSchema[]
  templateGitAuth?: TypesGen.TemplateVersionGitAuth[]
  createWorkspaceErrors: Partial<Record<CreateWorkspaceErrors, Error | unknown>>
  canCreateForUser?: boolean
  owner: TypesGen.User | null
  setOwner: (arg0: TypesGen.User | null) => void
  onCancel: () => void
  onSubmit: (req: TypesGen.CreateWorkspaceRequest) => void
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<TypesGen.CreateWorkspaceRequest>
  defaultParameterValues?: Record<string, string>
}

export const CreateWorkspacePageView: FC<
  React.PropsWithChildren<CreateWorkspacePageViewProps>
> = (props) => {
  const [parameterValues, setParameterValues] = useState<
    Record<string, string>
  >(props.defaultParameterValues ?? {})
  const initialRichParameterValues = selectInitialRichParametersValues(
    props.templateParameters,
    props.defaultParameterValues,
  )
  const [gitAuthErrors, setGitAuthErrors] = useState<Record<string, string>>({})
  useEffect(() => {
    // templateGitAuth is refreshed automatically using a BroadcastChannel
    // which may change the `authenticated` property.
    //
    // If the provider becomes authenticated, we want the error message
    // to disappear.
    setGitAuthErrors({})
  }, [props.templateGitAuth])

  const workspaceErrors =
    props.createWorkspaceErrors[CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]

  // Scroll to top of page if errors are present
  useEffect(() => {
    if (props.hasTemplateErrors || Boolean(workspaceErrors)) {
      window.scrollTo(0, 0)
    }
  }, [props.hasTemplateErrors, workspaceErrors])

  const { t } = useTranslation("createWorkspacePage")
  const styles = useStyles()

  const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
    useFormik<TypesGen.CreateWorkspaceRequest>({
      initialValues: {
        name: props.name,
        template_id: props.selectedTemplate ? props.selectedTemplate.id : "",
        rich_parameter_values: initialRichParameterValues,
      },
      validationSchema: Yup.object({
        name: nameValidator(t("nameLabel", { ns: "createWorkspacePage" })),
        rich_parameter_values: useValidationSchemaForRichParameters(
          "createWorkspacePage",
          props.templateParameters,
        ),
      }),
      enableReinitialize: true,
      initialTouched: props.initialTouched,
      onSubmit: (request) => {
        for (let i = 0; i < (props.templateGitAuth?.length || 0); i++) {
          const auth = props.templateGitAuth?.[i]
          if (!auth) {
            continue
          }
          if (!auth.authenticated) {
            setGitAuthErrors({
              [auth.id]: "You must authenticate to create a workspace!",
            })
            form.setSubmitting(false)
            return
          }
        }

        if (!props.templateSchema) {
          throw new Error("No template schema loaded")
        }

        const createRequests: TypesGen.CreateParameterRequest[] = []
        props.templateSchema.forEach((schema) => {
          let value = schema.default_source_value
          if (schema.name in parameterValues) {
            value = parameterValues[schema.name]
          }
          createRequests.push({
            name: schema.name,
            destination_scheme: schema.default_destination_scheme,
            source_scheme: "data",
            source_value: value,
          })
        })
        props.onSubmit({
          ...request,
          parameter_values: createRequests,
        })
        form.setSubmitting(false)
      },
    })

  const isLoading = props.loadingTemplateSchema || props.loadingTemplates
  // We only want to show schema that have redisplay_value equals true
  const schemaToBeDisplayed = props.templateSchema?.filter(
    (schema) => schema.redisplay_value,
  )

  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceRequest>(
    form,
    props.createWorkspaceErrors[CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR],
  )

  if (isLoading) {
    return <Loader />
  }

  return (
    <FullPageHorizontalForm title="New workspace" onCancel={props.onCancel}>
      <HorizontalForm onSubmit={form.handleSubmit}>
        {Boolean(props.hasTemplateErrors) && (
          <Stack>
            {Boolean(
              props.createWorkspaceErrors[
                CreateWorkspaceErrors.GET_TEMPLATES_ERROR
              ],
            ) && (
              <ErrorAlert
                error={
                  props.createWorkspaceErrors[
                    CreateWorkspaceErrors.GET_TEMPLATES_ERROR
                  ]
                }
              />
            )}
            {Boolean(
              props.createWorkspaceErrors[
                CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR
              ],
            ) && (
              <ErrorAlert
                error={
                  props.createWorkspaceErrors[
                    CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR
                  ]
                }
              />
            )}
            {Boolean(
              props.createWorkspaceErrors[
                CreateWorkspaceErrors.GET_TEMPLATE_GITAUTH_ERROR
              ],
            ) && (
              <ErrorAlert
                error={
                  props.createWorkspaceErrors[
                    CreateWorkspaceErrors.GET_TEMPLATE_GITAUTH_ERROR
                  ]
                }
              />
            )}
          </Stack>
        )}

        {Boolean(
          props.createWorkspaceErrors[
            CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR
          ],
        ) && (
          <ErrorAlert
            error={
              props.createWorkspaceErrors[
                CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR
              ]
            }
          />
        )}

        {/* General info */}
        <FormSection
          title="General info"
          description="The template and name of your new workspace."
        >
          <FormFields>
            {props.selectedTemplate && (
              <SelectedTemplate template={props.selectedTemplate} />
            )}

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

        {/* Workspace owner */}
        {props.canCreateForUser && (
          <FormSection
            title="Workspace owner"
            description="The user that is going to own this workspace. If you are admin, you can create workspace for others."
          >
            <FormFields>
              <UserAutocomplete
                value={props.owner}
                onChange={props.setOwner}
                label={t("ownerLabel")}
                size="medium"
              />
            </FormFields>
          </FormSection>
        )}

        {/* Template git auth */}
        {props.templateGitAuth && props.templateGitAuth.length > 0 && (
          <FormSection
            title="Git Authentication"
            description="This template requires authentication to automatically perform Git operations on create."
          >
            <FormFields>
              {props.templateGitAuth.map((auth, index) => (
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

        {/* Template params */}
        {schemaToBeDisplayed && schemaToBeDisplayed.length > 0 && (
          <FormSection
            title="Template params"
            description="These values are provided by your template's Terraform configuration."
          >
            <FormFields>
              {schemaToBeDisplayed.map((schema) => (
                <ParameterInput
                  disabled={form.isSubmitting}
                  key={schema.id}
                  defaultValue={parameterValues[schema.name]}
                  onChange={(value) => {
                    setParameterValues({
                      ...parameterValues,
                      [schema.name]: value,
                    })
                  }}
                  schema={schema}
                />
              ))}
            </FormFields>
          </FormSection>
        )}

        {props.templateParameters && (
          <>
            <MutableTemplateParametersSection
              templateParameters={props.templateParameters}
              getInputProps={(parameter, index) => {
                return {
                  ...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  ),
                  onChange: (value) => {
                    form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    })
                  },
                  initialValue: workspaceBuildParameterValue(
                    initialRichParameterValues,
                    parameter,
                  ),
                  disabled: form.isSubmitting,
                }
              }}
            />
            <ImmutableTemplateParametersSection
              templateParameters={props.templateParameters}
              classes={{ root: styles.warningSection }}
              getInputProps={(parameter, index) => {
                return {
                  ...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  ),
                  onChange: (value) => {
                    form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    })
                  },
                  initialValue: workspaceBuildParameterValue(
                    initialRichParameterValues,
                    parameter,
                  ),
                  disabled: form.isSubmitting,
                }
              }}
            />
          </>
        )}

        <FormFooter
          onCancel={props.onCancel}
          isLoading={props.creatingWorkspace}
          submitLabel={t("createWorkspace")}
        />
      </HorizontalForm>
    </FullPageHorizontalForm>
  )
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
