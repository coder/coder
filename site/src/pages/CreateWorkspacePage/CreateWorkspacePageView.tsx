import MenuItem from "@material-ui/core/MenuItem"
import TextField, { TextFieldProps } from "@material-ui/core/TextField"
import { FormikContextType, useFormik } from "formik"
import { FC, useState } from "react"
import * as Yup from "yup"
import * as TypesGen from "../../api/typesGenerated"
import { FormFooter } from "../../components/FormFooter/FormFooter"
import { FullPageForm } from "../../components/FullPageForm/FullPageForm"
import { Loader } from "../../components/Loader/Loader"
import { Margins } from "../../components/Margins/Margins"
import { ParameterInput } from "../../components/ParameterInput/ParameterInput"
import { Stack } from "../../components/Stack/Stack"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "../../util/formUtils"

export const Language = {
  templateLabel: "Template",
  nameLabel: "Name",
}

export interface CreateWorkspacePageViewProps {
  loadingTemplates: boolean
  loadingTemplateSchema: boolean
  creatingWorkspace: boolean
  templates?: TypesGen.Template[]
  selectedTemplate?: TypesGen.Template
  templateSchema?: TypesGen.ParameterSchema[]
  onCancel: () => void
  onSubmit: (req: TypesGen.CreateWorkspaceRequest) => void
  onSelectTemplate: (template: TypesGen.Template) => void
}

export const validationSchema = Yup.object({
  name: nameValidator(Language.nameLabel),
})

export const CreateWorkspacePageView: FC<CreateWorkspacePageViewProps> = (props) => {
  const [parameterValues, setParameterValues] = useState<Record<string, string>>({})
  const form: FormikContextType<TypesGen.CreateWorkspaceRequest> = useFormik<TypesGen.CreateWorkspaceRequest>({
    initialValues: {
      name: "",
      template_id: props.selectedTemplate ? props.selectedTemplate.id : "",
    },
    enableReinitialize: true,
    validationSchema,
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
          source_scheme: schema.default_source_scheme,
          source_value: value,
        })
      })
      return props.onSubmit({
        ...request,
        parameter_values: createRequests,
      })
    },
  })
  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceRequest>(form)

  const handleTemplateChange: TextFieldProps["onChange"] = (event) => {
    if (!props.templates) {
      throw new Error("Templates are not loaded")
    }

    const templateId = event.target.value
    const selectedTemplate = props.templates.find((template) => template.id === templateId)

    if (!selectedTemplate) {
      throw new Error(`Template ${templateId} not found`)
    }

    form.setFieldValue("template_id", selectedTemplate.id)
    props.onSelectTemplate(selectedTemplate)
  }

  return (
    <Margins>
      <FullPageForm title="Create workspace" onCancel={props.onCancel}>
        <form onSubmit={form.handleSubmit}>
          {props.loadingTemplates && <Loader />}

          <Stack>
            {props.templates && (
              <TextField
                {...getFieldHelpers("template_id")}
                disabled={form.isSubmitting}
                onChange={handleTemplateChange}
                autoFocus
                fullWidth
                label={Language.templateLabel}
                variant="outlined"
                select
              >
                {props.templates.map((template) => (
                  <MenuItem key={template.id} value={template.id}>
                    {template.name}
                  </MenuItem>
                ))}
              </TextField>
            )}

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
    </Margins>
  )
}
