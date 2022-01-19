import React from "react"

import { useRouter } from "next/router"
import { FormPage, FormButton } from "../../../components/Page"
import { useRequestor } from "../../../hooks/useRequest"
import * as Api from "./../../../api"
import CircularProgress from "@material-ui/core/CircularProgress"
import { ProjectIcon } from "../../../components/Project/ProjectIcon"
import Box from "@material-ui/core/Box"

const CreateSelectProjectPage: React.FC = () => {
  const router = useRouter()
  const requestState = useRequestor(() => Api.Project.get("test-org"))

  const cancel = () => {
    router.back()
  }

  const select = (projectId: string) => () => {
    router.push(`/workspaces/create/${projectId}`)
  }

  let body

  switch (requestState.state) {
    case "loading":
      body = <CircularProgress />
      break
    case "error":
      body = <>{requestState.error.toString()}</>
      break
    case "success":
      body = (
        <>
          {requestState.payload.map((project) => {
            return <ProjectIcon title={project.name} icon={project.icon} onClick={select(project.id)} />
          })}
        </>
      )
      break
  }

  const buttons: FormButton[] = [
    {
      title: "Cancel",
      props: {
        variant: "outlined",
        onClick: cancel,
      },
    },
  ]

  return (
    <FormPage title={"Select Project"} organization={"test-org"} buttons={buttons}>
      <Box style={{ display: "flex", flexDirection: "row", justifyContent: "center", alignItems: "center" }}>
        {body}
      </Box>
    </FormPage>
  )
}

export default CreateSelectProjectPage
