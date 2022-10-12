import TextField from "@material-ui/core/TextField"
import { CreateGroupRequest } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Margins } from "components/Margins/Margins"
import { useFormik } from "formik"
import React from "react"
import { useNavigate } from "react-router-dom"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"

const validationSchema = Yup.object({
  name: nameValidator("Name"),
})

export type CreateGroupPageViewProps = {
  onSubmit: (data: CreateGroupRequest) => void
  formErrors: unknown | undefined
  isLoading: boolean
}

export const CreateGroupPageView: React.FC<CreateGroupPageViewProps> = ({
  onSubmit,
  formErrors,
  isLoading,
}) => {
  const navigate = useNavigate()
  const form = useFormik<CreateGroupRequest>({
    initialValues: {
      name: "",
    },
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<CreateGroupRequest>(form, formErrors)
  const onCancel = () => navigate("/groups")

  return (
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
          <FormFooter onCancel={onCancel} isLoading={isLoading} />
        </form>
      </FullPageForm>
    </Margins>
  )
}
export default CreateGroupPageView
