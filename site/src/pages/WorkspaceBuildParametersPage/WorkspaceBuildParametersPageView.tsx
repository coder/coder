import { FC } from "react"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { useTranslation } from "react-i18next"
import * as TypesGen from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Stack } from "components/Stack/Stack"
import { makeStyles } from "@material-ui/core/styles"
import { getFormHelpers } from "util/formUtils"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput"
import {
  ValidationSchemaForRichParameters,
  workspaceBuildParameterValue,
} from "pages/CreateWorkspacePage/CreateWorkspacePageView"
import { FormFooter } from "components/FormFooter/FormFooter"
import * as Yup from "yup"
import { Maybe } from "components/Conditionals/Maybe"
import { GoBackButton } from "components/GoBackButton/GoBackButton"

export enum UpdateWorkspaceErrors {
  GET_WORKSPACE_ERROR = "getWorkspaceError",
  GET_TEMPLATE_PARAMETERS_ERROR = "getTemplateParametersError",
  GET_WORKSPACE_BUILD_PARAMETERS_ERROR = "getWorkspaceBuildParametersError",
  UPDATE_WORKSPACE_ERROR = "updateWorkspaceError",
}

export interface WorkspaceBuildParametersPageViewProps {
  workspace?: TypesGen.Workspace
  templateParameters?: TypesGen.TemplateVersionParameter[]
  workspaceBuildParameters?: TypesGen.WorkspaceBuildParameter[]

  isLoading: boolean
  initialTouched?: FormikTouched<TypesGen.CreateWorkspaceRequest>
  updatingWorkspace: boolean
  onCancel: () => void
  onSubmit: (req: TypesGen.CreateWorkspaceBuildRequest) => void

  hasErrors: boolean
  updateWorkspaceErrors: Partial<Record<UpdateWorkspaceErrors, Error | unknown>>
}

export const WorkspaceBuildParametersPageView: FC<
  React.PropsWithChildren<WorkspaceBuildParametersPageViewProps>
> = (props) => {
  const { t } = useTranslation("workspaceBuildParametersPage")
  const styles = useStyles()
  const formFooterStyles = useFormFooterStyles()

  const initialRichParameterValues = selectInitialRichParametersValues(
    props.templateParameters,
    props.workspaceBuildParameters,
  )

  const form: FormikContextType<TypesGen.CreateWorkspaceBuildRequest> =
    useFormik<TypesGen.CreateWorkspaceBuildRequest>({
      initialValues: {
        template_version_id: props.workspace
          ? props.workspace.latest_build.template_version_id
          : "",
        transition: "start",
        rich_parameter_values: initialRichParameterValues,
      },
      validationSchema: Yup.object({
        rich_parameter_values: ValidationSchemaForRichParameters(
          "workspaceBuildParametersPage",
          props.templateParameters,
          initialRichParameterValues,
        ),
      }),
      enableReinitialize: true,
      initialTouched: props.initialTouched,
      onSubmit: (request) => {
        props.onSubmit(
          stripImmutableParameters(request, props.templateParameters),
        )
        form.setSubmitting(false)
      },
    })

  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceBuildRequest>(
    form,
    props.updateWorkspaceErrors[UpdateWorkspaceErrors.UPDATE_WORKSPACE_ERROR],
  )

  {
    props.hasErrors && (
      <Stack>
        {Boolean(
          props.updateWorkspaceErrors[
            UpdateWorkspaceErrors.GET_WORKSPACE_ERROR
          ],
        ) && (
          <AlertBanner
            severity="error"
            error={
              props.updateWorkspaceErrors[
                UpdateWorkspaceErrors.GET_WORKSPACE_ERROR
              ]
            }
          />
        )}
        {Boolean(
          props.updateWorkspaceErrors[
            UpdateWorkspaceErrors.GET_TEMPLATE_PARAMETERS_ERROR
          ],
        ) && (
          <AlertBanner
            severity="error"
            error={
              props.updateWorkspaceErrors[
                UpdateWorkspaceErrors.GET_TEMPLATE_PARAMETERS_ERROR
              ]
            }
          />
        )}
        {Boolean(
          props.updateWorkspaceErrors[
            UpdateWorkspaceErrors.GET_WORKSPACE_BUILD_PARAMETERS_ERROR
          ],
        ) && (
          <AlertBanner
            severity="error"
            error={
              props.updateWorkspaceErrors[
                UpdateWorkspaceErrors.GET_WORKSPACE_BUILD_PARAMETERS_ERROR
              ]
            }
          />
        )}
      </Stack>
    )
  }

  return (
    <FullPageForm title={t("title")} detail={t("detail")}>
      <Maybe
        condition={Boolean(
          props.updateWorkspaceErrors[
            UpdateWorkspaceErrors.UPDATE_WORKSPACE_ERROR
          ],
        )}
      >
        <AlertBanner
          severity="error"
          error={
            props.updateWorkspaceErrors[
              UpdateWorkspaceErrors.UPDATE_WORKSPACE_ERROR
            ]
          }
        />
      </Maybe>

      <Maybe
        condition={Boolean(
          !props.isLoading &&
            props.templateParameters &&
            props.templateParameters.length === 0,
        )}
      >
        <div className={styles.formSection}>
          <AlertBanner severity="info" text={t("noParametersDefined")} />
          <div className={styles.goBackSection}>
            <GoBackButton onClick={props.onCancel} />
          </div>
        </div>
      </Maybe>

      {!props.isLoading &&
        props.templateParameters &&
        props.templateParameters.length > 0 &&
        props.workspaceBuildParameters && (
          <div className={styles.formSection}>
            <form onSubmit={form.handleSubmit}>
              <Stack
                direction="column"
                spacing={4} // Spacing here is diff because the fields here don't have the MUI floating label spacing
                className={styles.formSectionFields}
              >
                {props.templateParameters.filter((p) => !p.mutable).length >
                  0 && (
                  <div className={styles.formSectionParameterTitle}>
                    Immutable parameters
                  </div>
                )}
                {props.templateParameters.map(
                  (parameter, index) =>
                    !parameter.mutable && (
                      <RichParameterInput
                        {...getFieldHelpers(
                          "rich_parameter_values[" + index + "].value",
                        )}
                        disabled={!parameter.mutable || form.isSubmitting}
                        index={index}
                        key={parameter.name}
                        onChange={(value) => {
                          form.setFieldValue("rich_parameter_values." + index, {
                            name: parameter.name,
                            value: value,
                          })
                        }}
                        parameter={parameter}
                        initialValue={workspaceBuildParameterValue(
                          initialRichParameterValues,
                          parameter,
                        )}
                      />
                    ),
                )}

                {props.templateParameters.filter((p) => p.mutable).length >
                  0 && (
                  <div className={styles.formSectionParameterTitle}>
                    Mutable parameters
                  </div>
                )}
                {props.templateParameters.map(
                  (parameter, index) =>
                    parameter.mutable && (
                      <RichParameterInput
                        {...getFieldHelpers(
                          "rich_parameter_values[" + index + "].value",
                        )}
                        disabled={!parameter.mutable || form.isSubmitting}
                        index={index}
                        key={parameter.name}
                        onChange={(value) => {
                          form.setFieldValue("rich_parameter_values." + index, {
                            name: parameter.name,
                            value: value,
                          })
                        }}
                        parameter={parameter}
                        initialValue={workspaceBuildParameterValue(
                          initialRichParameterValues,
                          parameter,
                        )}
                      />
                    ),
                )}
                <FormFooter
                  styles={formFooterStyles}
                  onCancel={props.onCancel}
                  isLoading={props.updatingWorkspace}
                  submitLabel={t("updateWorkspace")}
                />
              </Stack>
            </form>
          </div>
        )}
    </FullPageForm>
  )
}

const selectInitialRichParametersValues = (
  templateParameters?: TypesGen.TemplateVersionParameter[],
  workspaceBuildParameters?: TypesGen.WorkspaceBuildParameter[],
): TypesGen.WorkspaceBuildParameter[] => {
  const defaults: TypesGen.WorkspaceBuildParameter[] = []
  if (!templateParameters) {
    return defaults
  }

  templateParameters.forEach((parameter) => {
    if (parameter.options.length > 0) {
      let parameterValue = parameter.options[0].value
      if (workspaceBuildParameters) {
        const foundBuildParameter = workspaceBuildParameters.find(
          (buildParameter) => {
            return buildParameter.name === parameter.name
          },
        )
        if (foundBuildParameter) {
          parameterValue = foundBuildParameter.value
        }
      }

      const buildParameter: TypesGen.WorkspaceBuildParameter = {
        name: parameter.name,
        value: parameterValue,
      }
      defaults.push(buildParameter)
      return
    }

    let parameterValue = parameter.default_value
    if (workspaceBuildParameters) {
      const foundBuildParameter = workspaceBuildParameters.find(
        (buildParameter) => {
          return buildParameter.name === parameter.name
        },
      )
      if (foundBuildParameter) {
        parameterValue = foundBuildParameter.value
      }
    }

    const buildParameter: TypesGen.WorkspaceBuildParameter = {
      name: parameter.name,
      value: parameterValue || "",
    }
    defaults.push(buildParameter)
  })
  return defaults
}

const stripImmutableParameters = (
  request: TypesGen.CreateWorkspaceBuildRequest,
  templateParameters?: TypesGen.TemplateVersionParameter[],
): TypesGen.CreateWorkspaceBuildRequest => {
  if (!templateParameters || !request.rich_parameter_values) {
    return request
  }

  const mutableBuildParameters = request.rich_parameter_values.filter(
    (buildParameter) =>
      templateParameters.find(
        (templateParameter) => templateParameter.name === buildParameter.name,
      )?.mutable,
  )

  return {
    ...request,
    rich_parameter_values: mutableBuildParameters,
  }
}

const useStyles = makeStyles((theme) => ({
  goBackSection: {
    display: "flex",
    width: "100%",
    marginTop: 32,
  },
  formSection: {
    marginTop: 20,
  },

  formSectionFields: {
    width: "100%",
  },
  formSectionParameterTitle: {
    fontSize: 20,
    color: theme.palette.text.primary,
    fontWeight: 400,
    margin: 0,
    marginBottom: theme.spacing(1),
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
