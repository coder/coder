import React from "react"
import { useRouter } from "next/router"
import { useFormik } from "formik"
import { firstOrOnly } from "./../../../util"

import * as API from "../../../api"

import { FormPage, FormButton } from "../../../components/PageTemplates"
import { useRequestor } from "../../../hooks/useRequest"
import { FormSection, FormRow } from "../../../components/Form"
import { formTextFieldFactory } from "../../../components/Form/FormTextField"
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

namespace Helpers {
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
  const router = useRouter()
  const { projectId: routeProjectId } = router.query
  const projectId = firstOrOnly(routeProjectId)

  const projectToLoad = useRequestor(() => API.Project.getProject("test-org", projectId), [projectId])

  const parametersWithMetadata = projectToLoad.state === "success" ? projectToLoad.payload.parameters : []
  const parameters = Helpers.projectParametersToValues(parametersWithMetadata)

  const form = useFormik({
    enableReinitialize: true,
    initialValues: {
      name: "",
      parameters,
    },
    onSubmit: async ({ name, parameters }) => {
      return API.Workspace.createWorkspace(name, projectId, parameters)
    },
  })

  const cancel = () => {
    router.push(`/workspaces/create`)
  }

  const submit = async () => {
    const workspaceId = await form.submitForm()
    router.push(`/workspaces/${workspaceId}`)
  }

  return (
    <LoadingPage request={projectToLoad}>
      {(project) => {
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
                    <FormTextField
                      form={form}
                      formFieldName={"parameters." + param.id}
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
    </LoadingPage>
  )
}

export default CreateProjectPage
