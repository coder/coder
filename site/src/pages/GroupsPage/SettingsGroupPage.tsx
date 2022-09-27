import TextField from "@material-ui/core/TextField"
import { useMachine } from "@xstate/react"
import { Group } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { FormFooter } from "components/FormFooter/FormFooter"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { Margins } from "components/Margins/Margins"
import { useFormik } from "formik"
import React from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
import { pageTitle } from "util/page"
import { editGroupMachine } from "xServices/groups/editGroupXService"
import * as Yup from "yup"

type FormData = {
  name: string
}

const validationSchema = Yup.object({
  name: nameValidator("Name"),
})

const UpdateGroupForm: React.FC<{
  group: Group
  errors: unknown
  onSubmit: (data: FormData) => void
  onCancel: () => void
  isLoading: boolean
}> = ({ group, errors, onSubmit, onCancel, isLoading }) => {
  const form = useFormik<FormData>({
    initialValues: {
      name: group.name,
    },
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<FormData>(form, errors)

  return (
    <FullPageForm title="Group settings" onCancel={onCancel}>
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
        <FormFooter onCancel={onCancel} isLoading={isLoading} />
      </form>
    </FullPageForm>
  )
}

export const SettingsGroupPage: React.FC = () => {
  const { groupId } = useParams()
  if (!groupId) {
    throw new Error("Group ID not defined.")
  }
  const navigate = useNavigate()
  const [editState, sendEditEvent] = useMachine(editGroupMachine, {
    context: {
      groupId,
    },
    actions: {
      onUpdate: () => {
        navigate(`/groups/${groupId}`)
      },
    },
  })
  const { updateGroupFormErrors, group } = editState.context

  const onCancel = () => {
    navigate("/groups")
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle("Settings Group")}</title>
      </Helmet>

      <ChooseOne>
        <Cond condition={editState.matches("loading")}>
          <FullScreenLoader />
        </Cond>

        <Cond condition>
          <Margins>
            <UpdateGroupForm
              group={group as Group}
              onCancel={onCancel}
              errors={updateGroupFormErrors}
              isLoading={editState.matches("updating")}
              onSubmit={(data) => {
                sendEditEvent({ type: "UPDATE", data })
              }}
            />
          </Margins>
        </Cond>
      </ChooseOne>
    </>
  )
}
export default SettingsGroupPage
