import { useMachine } from "@xstate/react"
import { FC, useEffect } from "react"
import { Helmet } from "react-helmet"
import { useSearchParams } from "react-router-dom"
import { pageTitle } from "../../util/page"
import { workspaceFilterQuery } from "../../util/workspace"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

const WorkspacesPage: FC = () => {
  const [workspacesState, send] = useMachine(workspacesMachine)
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const filter = searchParams.get("filter")
    const query = filter !== null ? filter : workspaceFilterQuery.me

    send({
      type: "SET_FILTER",
      query,
    })
  }, [searchParams, send])

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        filter={workspacesState.context.filter}
        loading={workspacesState.hasTag("loading")}
        workspaces={workspacesState.context.workspaces}
        onFilter={(query) => {
          searchParams.set("filter", query)
          setSearchParams(searchParams)
        }}
      />
    </>
  )
}

export default WorkspacesPage
