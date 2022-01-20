import React from "react"
import { useRouter } from "next/router"
import { useFormik } from "formik"
import { firstOrOnly, subForm, FormikLike } from "./../../../util"
import * as API from "../../../api"
import { FormPage, FormButton } from "../../../components/PageTemplates"
import { useRequestor } from "../../../hooks/useRequestor"
import { FormSection, FormRow, formTextFieldFactory } from "../../../components/Form"
import { LoadingPage } from "../../../components/PageTemplates/LoadingPage"

namespace CreateProjectForm {
  export interface Schema {
    name: string
    parameters: Record<string, string>
  }

  export const initial: Schema = {
    name: "",
    parameters: {},
  }
}

const FormTextField = formTextFieldFactory<CreateProjectForm.Schema>()
const ParameterTextField = formTextFieldFactory<Record<string, string>>()

namespace Helpers {
  /**
   * Convert an array for project paremeters to an id -> value dictionary
   *
   * @param parameters An array of `ProjectParameter`
   * @returns A `Record<string, string>`, where the key is a parameter id, and the value is the default value
   */
  export const projectParametersToValues = (parameters: API.ProjectParameter[]) => {
    const parameterValues: Record<string, string> = {}
    return parameters.reduce((acc, curr) => {
      return {
        ...acc,
        [curr.id]: curr.defaultValue || "",
      }
    }, parameterValues)
  }
}

const CreateProjectPage: React.FC = () => {
  // Grab the `projectId` from a route
  const router = useRouter()
  const { projectId: routeProjectId } = router.query
  // ...there can be more than one specified, but we don't handle that case.
  const projectId = firstOrOnly(routeProjectId)

  const projectToLoad = useRequestor(() => API.Project.getProject("test-org", projectId), [projectId])

  // When the project is loaded, we need to pluck the default parameters out and hand them off to formik
  const parametersWithMetadata = projectToLoad.state === "success" ? projectToLoad.payload.parameters : []
  const parameters = Helpers.projectParametersToValues(parametersWithMetadata)

  const form = useFormik({
    enableReinitialize: true,
    // TODO: Set up validation, based on form fields that come from ProjectParameters
    initialValues: {
      name: "",
      parameters,
    },
    onSubmit: async ({ name, parameters }) => {
      return API.Workspace.createWorkspace(name, projectId, parameters)
    },
  })

  const parametersForm: FormikLike<Record<string, string>> = subForm(form, "parameters")

  const cancel = () => {
    router.push(`/workspaces/create`)
  }

  const submit = async () => {
    const workspaceId = await form.submitForm()
    router.push(`/workspaces/${workspaceId}`)
  }

  return (
    <LoadingPage<API.Project>
      request={projectToLoad}
      render={(project) => {
        const buttons: FormButton[] = [
          {
            title: "Back",
            props: {
              variant: "outlined",
              onClick: cancel,
            },
          },
          {
            title: "Create Workspace",
            props: {
              variant: "contained",
              color: "primary",
              disabled: false,
              type: "submit",
              onClick: submit,
            },
          },
        ]

        const detail = (
          <>
            <strong>{project.name}</strong> in <strong> {"test-org"}</strong> organization
          </>
        )
        return (
          <FormPage title={"Create Project"} detail={detail} buttons={buttons}>
            <FormSection title="General">
              <FormRow>
                <FormTextField
                  form={form}
                  formFieldName="name"
                  fullWidth
                  helperText="A unique name describing your workspace."
                  label="Workspace Name"
                  placeholder={project.id}
                  required
                />
              </FormRow>
            </FormSection>
            <FormSection title="Parameters">
              {parametersWithMetadata.map((param) => {
                return (
                  <FormRow>
                    <ParameterTextField
                      form={parametersForm}
                      formFieldName={param.id}
                      fullWidth
                      label={param.name}
                      helperText={param.description}
                      placeholder={param.defaultValue}
                      required
                    />
                  </FormRow>
                )
              })}
            </FormSection>
          </FormPage>
        )
      }}
    />
  )
}

export default CreateProjectPage
