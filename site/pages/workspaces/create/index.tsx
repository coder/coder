import Typography from "@material-ui/core/Typography"
import React from "react"

import { Title } from "../../../components/FullScreenForm"

const CreateSelectProjectPage: React.FC = () => {

  const createWorkspace = () => {
    alert("create")
  }

  const button = {
    children: "Next",
    onClick: createWorkspace,
  }

  return (
    <Title title={"Select Project"} organization={"test-org"} />
  )
}

export default CreateSelectProjectPage