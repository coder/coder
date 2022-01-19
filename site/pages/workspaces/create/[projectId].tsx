import React from "react"
import { useRouter } from "next/router"

import { FormPage, FormButton } from "../../../components/PageTemplates"

const CreateProjectPage: React.FC = () => {
  const router = useRouter()
  const { projectId } = router.query

  const cancel = () => {
    router.back()
  }

  const submit = () => {
    alert("Submitting workspace")
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
      <div>TODO: Dynamic form fields</div>
    </FormPage>
  )
}

export default CreateProjectPage
