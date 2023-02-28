import TextField from "@material-ui/core/TextField"
import * as TypesGen from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { ParameterInput } from "components/ParameterInput/ParameterInput"
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput"
import { Stack } from "components/Stack/Stack"
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { makeStyles } from "@material-ui/core/styles"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { SelectedTemplate } from "./SelectedTemplate"
import { Loader } from "components/Loader/Loader"
import { GitAuth } from "components/GitAuth/GitAuth"

export enum CreateWorkspaceErrors {
  GET_TEMPLATES_ERROR = "getTemplatesError",
  GET_TEMPLATE_SCHEMA_ERROR = "getTemplateSchemaError",
  GET_TEMPLATE_GITAUTH_ERROR = "getTemplateGitAuthError",
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
  const styles = useStyles()
  const formFooterStyles = useFormFooterStyles()
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

  const { t } = useTranslation("createWorkspacePage")

  const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
    useFormik<TypesGen.CreateWorkspaceRequest>({
      initialValues: {
        name: "",
        template_id: props.selectedTemplate ? props.selectedTemplate.id : "",
        rich_parameter_values: initialRichParameterValues,
      },
      validationSchema: Yup.object({
        name: nameValidator(t("nameLabel", { ns: "createWorkspacePage" })),
        rich_parameter_values: ValidationSchemaForRichParameters(
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
        {Boolean(
          props.createWorkspaceErrors[
            CreateWorkspaceErrors.GET_TEMPLATE_GITAUTH_ERROR
          ],
        ) && (
          <AlertBanner
            severity="error"
            error={
              props.createWorkspaceErrors[
                CreateWorkspaceErrors.GET_TEMPLATE_GITAUTH_ERROR
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
                />
              </Stack>
            </div>
          )}

          {/* Template git auth */}
          {props.templateGitAuth && props.templateGitAuth.length > 0 && (
            <div className={styles.formSection}>
              <div className={styles.formSectionInfo}>
                <h2 className={styles.formSectionInfoTitle}>
                  Git Authentication
                </h2>
                <p className={styles.formSectionInfoDescription}>
                  This template requires authentication to automatically perform
                  Git operations on create.
                </p>
              </div>

              <Stack
                direction="column"
                spacing={2}
                className={styles.formSectionFields}
              >
                {props.templateGitAuth.map((auth, index) => (
                  <GitAuth
                    key={index}
                    authenticateURL={auth.authenticate_url}
                    authenticated={auth.authenticated}
                    type={auth.type}
                    error={gitAuthErrors[auth.id]}
                  />
                ))}
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
                {props.templateSchema
                  // We only want to show schema that have redisplay_value equals true
                  .filter((schema) => schema.redisplay_value)
                  .map((schema) => (
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

          {/* Immutable rich parameters */}
          {props.templateParameters &&
            props.templateParameters.filter((p) => !p.mutable).length > 0 && (
              <div className={styles.formSection}>
                <div className={styles.formSectionInfo}>
                  <h2 className={styles.formSectionInfoTitle}>
                    Immutable parameters
                  </h2>
                  <p className={styles.formSectionInfoDescription}>
                    Those values are provided by your template&lsquo;s Terraform
                    configuration. Values cannot be changed after creating the
                    workspace.
                  </p>
                </div>

                <Stack
                  direction="column"
                  spacing={4} // Spacing here is diff because the fields here don't have the MUI floating label spacing
                  className={styles.formSectionFields}
                >
                  {props.templateParameters.map(
                    (parameter, index) =>
                      !parameter.mutable && (
                        <RichParameterInput
                          {...getFieldHelpers(
                            "rich_parameter_values[" + index + "].value",
                          )}
                          disabled={form.isSubmitting}
                          index={index}
                          key={parameter.name}
                          onChange={(value) => {
                            form.setFieldValue(
                              "rich_parameter_values." + index,
                              {
                                name: parameter.name,
                                value: value,
                              },
                            )
                          }}
                          parameter={parameter}
                          initialValue={workspaceBuildParameterValue(
                            initialRichParameterValues,
                            parameter,
                          )}
                        />
                      ),
                  )}
                </Stack>
              </div>
            )}

          {/* Mutable rich parameters */}
          {props.templateParameters &&
            props.templateParameters.filter((p) => p.mutable).length > 0 && (
              <div className={styles.formSection}>
                <div className={styles.formSectionInfo}>
                  <h2 className={styles.formSectionInfoTitle}>
                    Mutable parameters
                  </h2>
                  <p className={styles.formSectionInfoDescription}>
                    Those values are provided by your template&lsquo;s Terraform
                    configuration. Values can be changed after creating the
                    workspace.
                  </p>
                </div>

                <Stack
                  direction="column"
                  spacing={4} // Spacing here is diff because the fields here don't have the MUI floating label spacing
                  className={styles.formSectionFields}
                >
                  {props.templateParameters.map(
                    (parameter, index) =>
                      parameter.mutable && (
                        <RichParameterInput
                          {...getFieldHelpers(
                            "rich_parameter_values[" + index + "].value",
                          )}
                          disabled={form.isSubmitting}
                          index={index}
                          key={parameter.name}
                          onChange={(value) => {
                            form.setFieldValue(
                              "rich_parameter_values." + index,
                              {
                                name: parameter.name,
                                value: value,
                              },
                            )
                          }}
                          parameter={parameter}
                          initialValue={workspaceBuildParameterValue(
                            initialRichParameterValues,
                            parameter,
                          )}
                        />
                      ),
                  )}
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

const selectInitialRichParametersValues = (
  templateParameters?: TypesGen.TemplateVersionParameter[],
  defaultValuesFromQuery?: Record<string, string>,
): TypesGen.WorkspaceBuildParameter[] => {
  const defaults: TypesGen.WorkspaceBuildParameter[] = []
  if (!templateParameters) {
    return defaults
  }

  templateParameters.forEach((parameter) => {
    if (parameter.options.length > 0) {
      let parameterValue = parameter.options[0].value
      if (defaultValuesFromQuery && defaultValuesFromQuery[parameter.name]) {
        parameterValue = defaultValuesFromQuery[parameter.name]
      }

      const buildParameter: TypesGen.WorkspaceBuildParameter = {
        name: parameter.name,
        value: parameterValue,
      }
      defaults.push(buildParameter)
      return
    }

    let parameterValue = parameter.default_value
    if (defaultValuesFromQuery && defaultValuesFromQuery[parameter.name]) {
      parameterValue = defaultValuesFromQuery[parameter.name]
    }

    const buildParameter: TypesGen.WorkspaceBuildParameter = {
      name: parameter.name,
      value: parameterValue || "",
    }
    defaults.push(buildParameter)
  })
  return defaults
}

export const workspaceBuildParameterValue = (
  workspaceBuildParameters: TypesGen.WorkspaceBuildParameter[],
  parameter: TypesGen.TemplateVersionParameter,
): string => {
  const buildParameter = workspaceBuildParameters.find((buildParameter) => {
    return buildParameter.name === parameter.name
  })
  return (buildParameter && buildParameter.value) || ""
}

export const ValidationSchemaForRichParameters = (
  ns: string,
  templateParameters?: TypesGen.TemplateVersionParameter[],
  lastBuildParameters?: TypesGen.WorkspaceBuildParameter[],
): Yup.AnySchema => {
  const { t } = useTranslation(ns)

  if (!templateParameters) {
    return Yup.object()
  }

  return Yup.array()
    .of(
      Yup.object().shape({
        name: Yup.string().required(),
        value: Yup.string()
          .required(t("validationRequiredParameter"))
          .test("verify with template", (val, ctx) => {
            const name = ctx.parent.name
            const templateParameter = templateParameters.find(
              (parameter) => parameter.name === name,
            )
            if (templateParameter) {
              switch (templateParameter.type) {
                case "number":
                  if (
                    templateParameter.validation_min &&
                    templateParameter.validation_max
                  ) {
                    if (
                      Number(val) < templateParameter.validation_min ||
                      templateParameter.validation_max < Number(val)
                    ) {
                      return ctx.createError({
                        path: ctx.path,
                        message: t("validationNumberNotInRange", {
                          min: templateParameter.validation_min,
                          max: templateParameter.validation_max,
                        }),
                      })
                    }
                  }

                  if (
                    templateParameter.validation_monotonic &&
                    lastBuildParameters
                  ) {
                    const lastBuildParameter = lastBuildParameters.find(
                      (last) => last.name === name,
                    )
                    if (lastBuildParameter) {
                      switch (templateParameter.validation_monotonic) {
                        case "increasing":
                          if (Number(lastBuildParameter.value) > Number(val)) {
                            return ctx.createError({
                              path: ctx.path,
                              message: t("validationNumberNotIncreasing", {
                                last: lastBuildParameter.value,
                              }),
                            })
                          }
                          break
                        case "decreasing":
                          if (Number(lastBuildParameter.value) < Number(val)) {
                            return ctx.createError({
                              path: ctx.path,
                              message: t("validationNumberNotDecreasing", {
                                last: lastBuildParameter.value,
                              }),
                            })
                          }
                          break
                      }
                    }
                  }
                  break
                case "string":
                  {
                    if (
                      !templateParameter.validation_regex ||
                      templateParameter.validation_regex.length === 0
                    ) {
                      return true
                    }

                    const regex = new RegExp(templateParameter.validation_regex)
                    if (val && !regex.test(val)) {
                      return ctx.createError({
                        path: ctx.path,
                        message: t("validationPatternNotMatched", {
                          error: templateParameter.validation_error,
                          pattern: templateParameter.validation_regex,
                        }),
                      })
                    }
                  }
                  break
              }
            }
            return true
          }),
      }),
    )
    .required()
}
