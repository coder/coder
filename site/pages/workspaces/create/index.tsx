import React from "react"

import { useRouter } from "next/router"
import { FormPage, FormButton } from "../../../components/PageTemplates"
import { useRequestor } from "../../../hooks/useRequest"
import * as Api from "./../../../api"
import CircularProgress from "@material-ui/core/CircularProgress"
import { ProjectIcon } from "../../../components/Project/ProjectIcon"
import Box from "@material-ui/core/Box"
import { LoadingPage } from "../../../components/PageTemplates/LoadingPage"

const CreateSelectProjectPage: React.FC = () => {
  const router = useRouter()
  const requestState = useRequestor(() => Api.Project.getAllProjectsInOrg("test-org"))

  const cancel = () => {
    router.push(`/workspaces`)
  }

  const select = (projectId: string) => () => {
    router.push(`/workspaces/create/${projectId}`)
  }

  return (
    <LoadingPage
      request={requestState}
      render={(projects) => {
        const buttons: FormButton[] = [
          {
            title: "Cancel",
            props: {
              variant: "outlined",
              onClick: cancel,
            },
          },
        ]
        const detail = (
          <>
            In <strong> {"test-org"}</strong> organization
          </>
        )
        return (
          <FormPage title={"Select Project"} detail={detail} buttons={buttons}>
            <Box style={{ display: "flex", flexDirection: "row", justifyContent: "center", alignItems: "center" }}>
              {projects.map((project: Api.Project) => {
                return <ProjectIcon title={project.name} icon={project.icon} onClick={select(project.id)} />
              })}
            </Box>
          </FormPage>
        )
      }}
    />
  )
}

export default CreateSelectProjectPage
