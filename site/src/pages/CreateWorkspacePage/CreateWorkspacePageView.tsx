import TextField from "@material-ui/core/TextField"
import * as TypesGen from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { ParameterInput } from "components/ParameterInput/ParameterInput"
import { Stack } from "components/Stack/Stack"
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { i18n } from "i18n"
import { FC, useState } from "react"
import { useTranslation } from "react-i18next"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { makeStyles } from "@material-ui/core/styles"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { SelectedTemplate } from "./SelectedTemplate"
import { Loader } from "components/Loader/Loader"

export enum CreateWorkspaceErrors {
  GET_TEMPLATES_ERROR = "getTemplatesError",
  GET_TEMPLATE_SCHEMA_ERROR = "getTemplateSchemaError",
  CREATE_WORKSPACE_ERROR = "createWorkspaceError",
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

const { t } = i18n

export const validationSchema = Yup.object({
  name: nameValidator(t("nameLabel", { ns: "createWorkspacePage" })),
})

export const CreateWorkspacePageView: FC<
  React.PropsWithChildren<CreateWorkspacePageViewProps>
> = (props) => {
  const { t } = useTranslation("createWorkspacePage")
  const styles = useStyles()
  const formFooterStyles = useFormFooterStyles()
  const [parameterValues, setParameterValues] = useState<
    Record<string, string>
  >(props.defaultParameterValues ?? {})

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

  const isLoading = props.loadingTemplateSchema || props.loadingTemplates

  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceRequest>(
    form,
    props.createWorkspaceErrors[CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR],
  )

  if (isLoading) {
    return <Loader />
  }

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

  if (
    props.createWorkspaceErrors[CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]
  ) {
    return (
      <AlertBanner
        severity="error"
        error={
          props.createWorkspaceErrors[
            CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR
          ]
        }
      />
    )
  }

  return (
    <FullPageHorizontalForm title="New workspace" onCancel={props.onCancel}>
      <form onSubmit={form.handleSubmit}>
        <Stack direction="column" spacing={10} className={styles.formSections}>
          {/* General info */}
          <div className={styles.formSection}>
            <div className={styles.formSectionInfo}>
              <h2 className={styles.formSectionInfoTitle}>General info</h2>
              <p className={styles.formSectionInfoDescription}>
                The template and name of your new workspace.
              </p>
            </div>

            <Stack
              direction="column"
              spacing={1}
              className={styles.formSectionFields}
            >
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
                variant="outlined"
              />
            </Stack>
          </div>

          {/* Workspace owner */}
          {props.canCreateForUser && (
            <div className={styles.formSection}>
              <div className={styles.formSectionInfo}>
                <h2 className={styles.formSectionInfoTitle}>Workspace owner</h2>
                <p className={styles.formSectionInfoDescription}>
                  The user that is going to own this workspace. If you are
                  admin, you can create workspace for others.
                </p>
              </div>

              <Stack
                direction="column"
                spacing={1}
                className={styles.formSectionFields}
              >
                <UserAutocomplete
                  value={props.owner}
                  onChange={props.setOwner}
                  label={t("ownerLabel")}
                  inputMargin="dense"
                  showAvatar
                />
              </Stack>
            </div>
          )}

          {/* Template params */}
          {props.templateSchema && props.templateSchema.length > 0 && (
            <div className={styles.formSection}>
              <div className={styles.formSectionInfo}>
                <h2 className={styles.formSectionInfoTitle}>Template params</h2>
                <p className={styles.formSectionInfoDescription}>
                  Those values are provided by your template&lsquo;s Terraform
                  configuration.
                </p>
              </div>

              <Stack
                direction="column"
                spacing={4} // Spacing here is diff because the fields here don't have the MUI floating label spacing
                className={styles.formSectionFields}
              >
                {props.templateSchema.map((schema) => (
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
              </Stack>
            </div>
          )}

          <FormFooter
            styles={formFooterStyles}
            onCancel={props.onCancel}
            isLoading={props.creatingWorkspace}
            submitLabel={t("createWorkspace")}
          />
        </Stack>
      </form>
    </FullPageHorizontalForm>
  )
}

const useStyles = makeStyles((theme) => ({
  formSections: {
    [theme.breakpoints.down("sm")]: {
      gap: theme.spacing(8),
    },
  },

  formSection: {
    display: "flex",
    alignItems: "flex-start",
    gap: theme.spacing(15),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      gap: theme.spacing(2),
    },
  },

  formSectionInfo: {
    width: 312,
    flexShrink: 0,
    position: "sticky",
    top: theme.spacing(3),

    [theme.breakpoints.down("sm")]: {
      width: "100%",
      position: "initial",
    },
  },

  formSectionInfoTitle: {
    fontSize: 20,
    color: theme.palette.text.primary,
    fontWeight: 400,
    margin: 0,
    marginBottom: theme.spacing(1),
  },

  formSectionInfoDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    margin: 0,
  },

  formSectionFields: {
    width: "100%",
  },
}))

const useFormFooterStyles = makeStyles((theme) => ({
  button: {
    minWidth: theme.spacing(23),

    [theme.breakpoints.down("sm")]: {
      width: "100%",
    },
  },
  footer: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-start",
    flexDirection: "row-reverse",
    gap: theme.spacing(2),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      gap: theme.spacing(1),
    },
  },
}))
