import TextField from "@material-ui/core/TextField"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import { UsersSelect } from "components/UsersSelect/UsersSelect"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC, useState } from "react"
import * as Yup from "yup"
import * as TypesGen from "../../api/typesGenerated"
import { FormFooter } from "../../components/FormFooter/FormFooter"
import { FullPageForm } from "../../components/FullPageForm/FullPageForm"
import { Loader } from "../../components/Loader/Loader"
import { ParameterInput } from "../../components/ParameterInput/ParameterInput"
import { Stack } from "../../components/Stack/Stack"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "../../util/formUtils"

export const Language = {
  templateLabel: "Template",
  nameLabel: "Name",
}

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
  defaultWorkspaceOwnerId?: string
  setOwnerId: (arg0: string) => void
  onCancel: () => void
  onSubmit: (req: TypesGen.CreateWorkspaceRequest) => void
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<TypesGen.CreateWorkspaceRequest>
}

export const validationSchema = Yup.object({
  name: nameValidator(Language.nameLabel),
})

export const CreateWorkspacePageView: FC<React.PropsWithChildren<CreateWorkspacePageViewProps>> = (
  props,
) => {
  const [parameterValues, setParameterValues] = useState<Record<string, string>>({})

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
        {props.createWorkspaceErrors[CreateWorkspaceErrors.GET_TEMPLATES_ERROR] ? (
          <ErrorSummary
            error={props.createWorkspaceErrors[CreateWorkspaceErrors.GET_TEMPLATES_ERROR]}
          />
        ) : (
          <></>
        )}
        {props.createWorkspaceErrors[CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR] ? (
          <ErrorSummary
            error={props.createWorkspaceErrors[CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR]}
          />
        ) : (
          <></>
        )}
      </Stack>
    )
  }

  return (
    <FullPageForm title="Create workspace" onCancel={props.onCancel}>
      <form onSubmit={form.handleSubmit}>
        <Stack>
          {Boolean(props.createWorkspaceErrors[CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]) && (
            <ErrorSummary
              error={props.createWorkspaceErrors[CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]}
            />
          )}
          <TextField
            disabled
            fullWidth
            label={Language.templateLabel}
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
                label={Language.nameLabel}
                variant="outlined"
              />

              {props.canCreateForUser && (
                <UsersSelect
                  defaultUserId={props.defaultWorkspaceOwnerId}
                  handleChange={(id: string) => props.setOwnerId(id)}
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

              <FormFooter onCancel={props.onCancel} isLoading={props.creatingWorkspace} />
            </>
          )}
        </Stack>
      </form>
    </FullPageForm>
  )
}
