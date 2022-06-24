import { useMachine } from "@xstate/react"
import { FC, useEffect } from "react"
import { Helmet } from "react-helmet"
import { useSearchParams } from "react-router-dom"
import { workspaceFilterQuery } from "../../util/filters"
import { pageTitle } from "../../util/page"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

const WorkspacesPage: FC = () => {
  const [workspacesState, send] = useMachine(workspacesMachine)
  const [searchParams, setSearchParams] = useSearchParams()
  const { workspaceRefs } = workspacesState.context

  useEffect(() => {
    const filter = searchParams.get("filter")
    const query = filter !== null ? filter : workspaceFilterQuery.me

    send({
      type: "GET_WORKSPACES",
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
        workspaceRefs={workspaceRefs}
        onFilter={(query) => {
          searchParams.set("filter", query)
          setSearchParams(searchParams)
        }}
      />
    </>
  )
}

export default WorkspacesPage
