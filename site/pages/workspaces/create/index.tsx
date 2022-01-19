import React from "react"

import { useRouter } from "next/router"
import { FormPage, FormButton } from "../../../components/Page"

const CreateSelectProjectPage: React.FC = () => {
  const router = useRouter()

  const createWorkspace = () => {
    alert("create")
  }

  const cancel = () => {
    router.back()
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
      title: "Next",
      props: {
        variant: "contained",
        color: "primary",
        disabled: false,
        type: "submit",
      },
    },
  ]

  return <FormPage title={"Select Project"} organization={"test-org"} buttons={buttons}></FormPage>
}

export default CreateSelectProjectPage
