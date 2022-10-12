import TextField from "@material-ui/core/TextField"
import * as TypesGen from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { ParameterInput } from "components/ParameterInput/ParameterInput"
import { Stack } from "components/Stack/Stack"
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete"
import { WorkspaceQuota } from "components/WorkspaceQuota/WorkspaceQuota"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { i18n } from "i18n"
import { FC, useState } from "react"
import { useTranslation } from "react-i18next"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"
import { AlertBanner } from "components/AlertBanner/AlertBanner"

export enum CreateWorkspaceErrors {
  GET_TEMPLATES_ERROR = "getTemplatesError",
  GET_TEMPLATE_SCHEMA_ERROR = "getTemplateSchemaError",
  CREATE_WORKSPACE_ERROR = "createWorkspaceError",
  GET_WORKSPACE_QUOTA_ERROR = "getWorkspaceQuotaError",
}

export interface CreateWorkspacePageViewProps {
  loadingTemplates: boolean
  loadingTemplateSchema: boolean
  creatingWorkspace: boolean
  hasTemplateErrors: boolean
  templateName: string
  templates?: TypesGen.Template[]
  selectedTemplate?: TypesGen.Template
  templateSchema?: TypesGen.ParameterSchema[]
  workspaceQuota?: TypesGen.WorkspaceQuota
  createWorkspaceErrors: Partial<Record<CreateWorkspaceErrors, Error | unknown>>
  canCreateForUser?: boolean
  owner: TypesGen.User | null
  setOwner: (arg0: TypesGen.User | null) => void
  onCancel: () => void
  onSubmit: (req: TypesGen.CreateWorkspaceRequest) => void
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<TypesGen.CreateWorkspaceRequest>
}

const { t } = i18n

export const validationSchema = Yup.object({
  name: nameValidator(t("nameLabel", { ns: "createWorkspacePage" })),
})

export const CreateWorkspacePageView: FC<
  React.PropsWithChildren<CreateWorkspacePageViewProps>
> = (props) => {
  const { t } = useTranslation("createWorkspacePage")

  const [parameterValues, setParameterValues] = useState<
    Record<string, string>
  >({})

  const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
    useFormik<TypesGen.CreateWorkspaceRequest>({
      initialValues: {
        name: "",
        template_id: props.selectedTemplate ? props.selectedTemplate.id : "",
      },
      enableReinitialize: true,
      validationSchema,
      initialTouched: props.initialTouched,
      onSubmit: (request) => {
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

  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceRequest>(
    form,
    props.createWorkspaceErrors[CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR],
  )

  if (props.hasTemplateErrors) {
    return (
      <Stack>
        {Boolean(
          props.createWorkspaceErrors[
            CreateWorkspaceErrors.GET_TEMPLATES_ERROR
          ],
        ) && (
          <AlertBanner
            severity="error"
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
          <AlertBanner
            severity="error"
            error={
              props.createWorkspaceErrors[
                CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR
              ]
            }
          />
        )}
      </Stack>
    )
  }

  const canSubmit =
    props.workspaceQuota && props.workspaceQuota.user_workspace_limit > 0
      ? props.workspaceQuota.user_workspace_count <
        props.workspaceQuota.user_workspace_limit
      : true

  return (
    <FullPageForm title="Create workspace" onCancel={props.onCancel}>
      <form onSubmit={form.handleSubmit}>
        <Stack>
          {Boolean(
            props.createWorkspaceErrors[
              CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR
            ],
          ) && (
            <AlertBanner
              severity="error"
              error={
                props.createWorkspaceErrors[
                  CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR
                ]
              }
            />
          )}
          <TextField
            disabled
            fullWidth
            label={t("templateLabel")}
            value={props.selectedTemplate?.name || props.templateName}
            variant="outlined"
          />

          {props.loadingTemplateSchema && <Loader />}
          {props.selectedTemplate && props.templateSchema && (
            <>
              <TextField
                {...getFieldHelpers("name")}
                disabled={form.isSubmitting}
                onChange={onChangeTrimmed(form)}
                autoFocus
                fullWidth
                label={t("nameLabel")}
                variant="outlined"
              />

              {props.canCreateForUser && (
                <UserAutocomplete
                  value={props.owner}
                  onChange={props.setOwner}
                  label={t("ownerLabel")}
                  inputMargin="dense"
                  showAvatar
                />
              )}

              {props.workspaceQuota && (
                <WorkspaceQuota
                  quota={props.workspaceQuota}
                  error={
                    props.createWorkspaceErrors[
                      CreateWorkspaceErrors.GET_WORKSPACE_QUOTA_ERROR
                    ]
                  }
                />
              )}

              {props.templateSchema.length > 0 && (
                <Stack>
                  {props.templateSchema.map((schema) => (
                    <ParameterInput
                      disabled={form.isSubmitting}
                      key={schema.id}
                      onChange={(value) => {
                        setParameterValues({
                          ...parameterValues,
                          [schema.name]: value,
                        })
                      }}
                      schema={schema}
                    />
                  ))}
                </Stack>
              )}

              <FormFooter
                onCancel={props.onCancel}
                isLoading={props.creatingWorkspace}
                submitDisabled={!canSubmit}
              />
            </>
          )}
        </Stack>
      </form>
    </FullPageForm>
  )
}
