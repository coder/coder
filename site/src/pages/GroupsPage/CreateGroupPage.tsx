import TextField from "@material-ui/core/TextField"
import { useMachine } from "@xstate/react"
import { CreateGroupRequest } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Margins } from "components/Margins/Margins"
import { useFormik } from "formik"
import { useOrganizationId } from "hooks/useOrganizationId"
import React from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate } from "react-router-dom"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
import { pageTitle } from "util/page"
import { createGroupMachine } from "xServices/groups/createGroupXService"
import * as Yup from "yup"

const validationSchema = Yup.object({
  name: nameValidator("Name"),
})

export const CreateGroupPage: React.FC = () => {
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
  const form = useFormik<CreateGroupRequest>({
    initialValues: {
      name: "",
    },
    validationSchema,
    onSubmit: (data) => {
      sendCreateEvent({
        type: "CREATE",
        data,
      })
    },
  })
  const getFieldHelpers = getFormHelpers<CreateGroupRequest>(form, createGroupFormErrors)
  const onCancel = () => navigate("/groups")

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Group")}</title>
      </Helmet>
      <Margins>
        <FullPageForm title="Create group" onCancel={onCancel}>
          <form onSubmit={form.handleSubmit}>
            <TextField
              {...getFieldHelpers("name")}
              onChange={onChangeTrimmed(form)}
              autoComplete="name"
              autoFocus
              fullWidth
              label="Name"
              variant="outlined"
            />
            <FormFooter onCancel={onCancel} isLoading={createState.matches("creatingGroup")} />
          </form>
        </FullPageForm>
      </Margins>
    </>
  )
}
export default CreateGroupPage
