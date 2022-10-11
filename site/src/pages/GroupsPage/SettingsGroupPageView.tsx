import TextField from "@material-ui/core/TextField"
import { Group } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { FormFooter } from "components/FormFooter/FormFooter"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { Margins } from "components/Margins/Margins"
import { useFormik } from "formik"
import React from "react"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
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

export type SettingsGroupPageViewProps = {
  onCancel: () => void
  onSubmit: (data: FormData) => void
  group: Group | undefined
  formErrors: unknown
  isLoading: boolean
  isUpdating: boolean
}

export const SettingsGroupPageView: React.FC<SettingsGroupPageViewProps> = ({
  onCancel,
  onSubmit,
  group,
  formErrors,
  isLoading,
  isUpdating,
}) => {
  return (
    <ChooseOne>
      <Cond condition={isLoading}>
        <FullScreenLoader />
      </Cond>

      <Cond>
        <Margins>
          <UpdateGroupForm
            group={group as Group}
            onCancel={onCancel}
            errors={formErrors}
            isLoading={isUpdating}
            onSubmit={onSubmit}
          />
        </Margins>
      </Cond>
    </ChooseOne>
  )
}

export default SettingsGroupPageView
