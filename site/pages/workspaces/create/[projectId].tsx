import React from "react"
import { useRouter } from "next/router"
import { useFormik } from "formik"
import { firstOrOnly } from "./../../../util"

import * as API from "../../../api"

import { FormPage, FormButton } from "../../../components/PageTemplates"
import { useRequestor } from "../../../hooks/useRequest"
import { FormSection } from "../../../components/Form"
import { formTextFieldFactory } from "../../../components/Form/FormTextField"

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
  const projectId = firstOrOnly(routeProjectId)
  // const projectToLoad = useRequestor(() => API.Project.getProject("test-org", projectId))

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
    <FormPage title={"Create Project"} organization={"test-org"} buttons={buttons}>
      <FormSection title="General">
        <FormTextField
          form={form}
          formFieldName="name"
          fullWidth
          helperText="A unique name describing your workspace."
          label="Workspace Name"
          placeholder="my-dev"
          required
        />
      </FormSection>
    </FormPage>
  )
}

export default CreateProjectPage
