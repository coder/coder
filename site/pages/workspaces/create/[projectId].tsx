import React from "react"
import { useRouter } from "next/router"

const CreateProjectPage: React.FC = () => {
  const router = useRouter()
  const { projectId } = router.query

  const createWorkspace = () => {
    alert("create")
  }

  const button = {
    children: "New Workspace",
    onClick: createWorkspace,
  }

  return <div>Create Page: {projectId}</div>
}

export default CreateProjectPage
