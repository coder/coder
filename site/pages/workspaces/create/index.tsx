import React from "react"

const CreateSelectProjectPage: React.FC = () => {

  const createWorkspace = () => {
    alert("create")
  }

  const button = {
    children: "New Workspace",
    onClick: createWorkspace,
  }

  return (
    <div>Create Page</div>
  )
}

export default CreateSelectProjectPage