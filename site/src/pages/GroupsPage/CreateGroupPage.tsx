import { useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate } from "react-router-dom"
import { pageTitle } from "util/page"
import { createGroupMachine } from "xServices/groups/createGroupXService"
import CreateGroupPageView from "./CreateGroupPageView"

export const CreateGroupPage: FC = () => {
  const navigate = useNavigate()
  const organizationId = useOrganizationId()
  const [createState, sendCreateEvent] = useMachine(createGroupMachine, {
    context: {
      organizationId,
    },
    actions: {
      onCreate: (_, { data }) => {
        navigate(`/groups/${data.id}`)
      },
    },
  })
  const { createGroupFormErrors } = createState.context

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Group")}</title>
      </Helmet>
      <CreateGroupPageView
        onSubmit={(data) => {
          sendCreateEvent({
            type: "CREATE",
            data,
          })
        }}
        formErrors={createGroupFormErrors}
        isLoading={createState.matches("creatingGroup")}
      />
    </>
  )
}
export default CreateGroupPage
