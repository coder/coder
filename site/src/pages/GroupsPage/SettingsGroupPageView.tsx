import TextField from "@material-ui/core/TextField"
import { Group } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { FormFooter } from "components/FormFooter/FormFooter"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { LazyIconField } from "components/IconField/LazyIconField"
import { Margins } from "components/Margins/Margins"
import { useFormik } from "formik"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { getFormHelpers, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"

type FormData = {
  name: string
  avatar_url: string
  quota_allowance: number
}

const validationSchema = Yup.object({
  name: nameValidator("Name"),
  quota_allowance: Yup.number().required().min(0).integer(),
})

const UpdateGroupForm: FC<{
  group: Group
  errors: unknown
  onSubmit: (data: FormData) => void
  onCancel: () => void
  isLoading: boolean
}> = ({ group, errors, onSubmit, onCancel, isLoading }) => {
  const form = useFormik<FormData>({
    initialValues: {
      name: group.name,
      avatar_url: group.avatar_url,
      quota_allowance: group.quota_allowance,
    },
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<FormData>(form, errors)
  const { t } = useTranslation("common")

  return (
    <FullPageForm title="Group settings">
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

        <LazyIconField
          {...getFieldHelpers("avatar_url")}
          onChange={onChangeTrimmed(form)}
          fullWidth
          label={t("form.fields.icon")}
          variant="outlined"
          onPickEmoji={(value) => form.setFieldValue("avatar_url", value)}
        />

        <TextField
          {...getFieldHelpers("quota_allowance")}
          onChange={onChangeTrimmed(form)}
          autoFocus
          fullWidth
          type="number"
          label="Quota Allowance"
          variant="outlined"
        />
        <span>
          This group gives {form.values.quota_allowance} quota credits to each
          of its members.
        </span>

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

export const SettingsGroupPageView: FC<SettingsGroupPageViewProps> = ({
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
        <Loader />
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
