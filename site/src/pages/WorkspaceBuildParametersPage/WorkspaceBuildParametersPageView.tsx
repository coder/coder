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

  initialTouched?: FormikTouched<TypesGen.CreateWorkspaceRequest>

  hasErrors: boolean
  updateWorkspaceErrors: Partial<Record<UpdateWorkspaceErrors, Error | unknown>>
}

export const WorkspaceBuildParametersPageView: FC<
  React.PropsWithChildren<WorkspaceBuildParametersPageViewProps>
> = (props) => {
  const { t } = useTranslation("workspaceBuildParametersPage")
  const styles = useStyles()

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
      validationSchema: ValidationSchemaForRichParameters(
        "workspaceBuildParametersPage",
        props.templateParameters,
      ),
      enableReinitialize: true,
      initialTouched: props.initialTouched,
      onSubmit: () => {
        form.setSubmitting(false)
      },
    })

  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceBuildRequest>(
    form,
    props.updateWorkspaceErrors[UpdateWorkspaceErrors.UPDATE_WORKSPACE_ERROR],
  )

  if (props.hasErrors) {
    return (
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

  if (
    props.updateWorkspaceErrors[UpdateWorkspaceErrors.UPDATE_WORKSPACE_ERROR]
  ) {
    return (
      <AlertBanner
        severity="error"
        error={
          props.updateWorkspaceErrors[
            UpdateWorkspaceErrors.UPDATE_WORKSPACE_ERROR
          ]
        }
      />
    )
  }

  return (
    <FullPageForm
      title={t("title")}
      detail="Those values are provided by your template‘s Terraform configuration."
    >
      {props.templateParameters && (
        <div className={styles.formSection}>
          <Stack
            direction="column"
            spacing={4} // Spacing here is diff because the fields here don't have the MUI floating label spacing
            className={styles.formSectionFields}
          >
            {props.templateParameters.map((parameter, index) => (
              <RichParameterInput
                {...getFieldHelpers(
                  "rich_parameter_values[" + index + "].value",
                )}
                disabled={form.isSubmitting}
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
            ))}
          </Stack>
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

const useStyles = makeStyles(() => ({
  formSection: {
    marginTop: 28,
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
