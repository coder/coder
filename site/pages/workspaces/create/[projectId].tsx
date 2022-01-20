import React from "react"
import { useRouter } from "next/router"
import { useFormik } from "formik"
import { firstOrOnly } from "./../../../util"

import * as API from "../../../api"

import { FormPage, FormButton } from "../../../components/PageTemplates"
import { useRequestor } from "../../../hooks/useRequest"
import { FormSection } from "../../../components/Form"
import { formTextFieldFactory } from "../../../components/Form/FormTextField"
import { LoadingPage } from "../../../components/PageTemplates/LoadingPage"

namespace CreateProjectForm {
  export interface Schema {
    name: string
    parameters: any[]
  }

  export const initial: Schema = {
    name: "",
    parameters: [],
  }
}

const FormTextField = formTextFieldFactory<CreateProjectForm.Schema>()

const CreateProjectPage: React.FC = () => {
  const router = useRouter()
  const { projectId: routeProjectId } = router.query
  console.log(routeProjectId)
  const projectId = firstOrOnly(routeProjectId)
  const projectToLoad = useRequestor(() => API.Project.getProject("test-org", projectId), [projectId])

  const form = useFormik({
    enableReinitialize: true,
    initialValues: CreateProjectForm.initial,
    onSubmit: async ({ name }) => {
      return API.Workspace.createWorkspace(name, projectId)
    },
  })

  const cancel = () => {
    router.back()
  }

  const submit = async () => {
    const workspaceId = await form.submitForm()
    router.push(`/workspaces/${workspaceId}`)
  }

  const buttons: FormButton[] = [
    {
      title: "Cancel",
      props: {
        variant: "outlined",
        onClick: cancel,
      },
    },
    {
      title: "Submit",
      props: {
        variant: "contained",
        color: "primary",
        disabled: false,
        type: "submit",
        onClick: submit,
      },
    },
  ]

  return (
    <LoadingPage request={projectToLoad}>
      {(project) => {
        const detail = (
          <>
            <strong>{project.name}</strong> in <strong> {"test-org"}</strong> organization
          </>
        )
        return (
          <FormPage title={"Create Project"} detail={detail} buttons={buttons}>
            <FormSection title="General">
              <FormTextField
                form={form}
                formFieldName="name"
                fullWidth
                helperText="A unique name describing your workspace."
                label="Workspace Name"
                placeholder={project.id}
                required
              />
            </FormSection>
          </FormPage>
        )
      }}
    </LoadingPage>
  )
}

export default CreateProjectPage
