import { FC, useMemo } from "react"
import { useParams } from "react-router-dom"
import { WorkspaceAppErrorPageView } from "./WorkspaceAppErrorPageView"

const WorkspaceAppErrorView: FC<React.PropsWithChildren<unknown>> = () => {
  const { app } = useParams()
  const message = useMemo(() => {
    const tag = document.getElementById("api-response")
    if (!tag) {
      throw new Error("dev error: api-response meta tag not found")
    }
    return tag.getAttribute("data-message") as string
  }, [])

  return <WorkspaceAppErrorPageView appName={app as string} message={message} />
}

export default WorkspaceAppErrorView
