import data from "@emoji-mart/data/sets/14/twitter.json"
import Picker from "@emoji-mart/react"
import Button from "@material-ui/core/Button"
import InputAdornment from "@material-ui/core/InputAdornment"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { Group } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { OpenDropdown } from "components/DropdownArrows/DropdownArrows"
import { FormFooter } from "components/FormFooter/FormFooter"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { Margins } from "components/Margins/Margins"
import { useFormik } from "formik"
import { useRef, useState, FC } from "react"
import { useTranslation } from "react-i18next"
import { colors } from "theme/colors"
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
  const [isEmojiPickerOpen, setIsEmojiPickerOpen] = useState(false)
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
  const hasIcon = form.values.avatar_url && form.values.avatar_url !== ""
  const emojiButtonRef = useRef<HTMLButtonElement>(null)
  const styles = useStyles()
  const { t } = useTranslation("common")

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
        <TextField
          {...getFieldHelpers("avatar_url")}
          onChange={onChangeTrimmed(form)}
          autoFocus
          fullWidth
          label="Icon"
          variant="outlined"
          InputProps={{
            endAdornment: hasIcon ? (
              <InputAdornment position="end">
                <img
                  alt=""
                  src={form.values.avatar_url}
                  className={styles.adornment}
                  // This prevent browser to display the ugly error icon if the
                  // image path is wrong or user didn't finish typing the url
                  onError={(e) => (e.currentTarget.style.display = "none")}
                  onLoad={(e) => (e.currentTarget.style.display = "inline")}
                />
              </InputAdornment>
            ) : undefined,
          }}
        />

        <Button
          fullWidth
          ref={emojiButtonRef}
          variant="outlined"
          size="small"
          endIcon={<OpenDropdown />}
          onClick={() => {
            setIsEmojiPickerOpen((v) => !v)
          }}
        >
          {t("emojiPicker.select")}
        </Button>

        <Popover
          id="emoji"
          open={isEmojiPickerOpen}
          anchorEl={emojiButtonRef.current}
          onClose={() => {
            setIsEmojiPickerOpen(false)
          }}
        >
          <Picker
            theme="dark"
            data={data}
            onEmojiSelect={(emojiData) => {
              form
                .setFieldValue("avatar_url", `/emojis/${emojiData.unified}.png`)
                .catch((ex) => {
                  console.error(ex)
                })
              setIsEmojiPickerOpen(false)
            }}
          />
        </Popover>

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

const useStyles = makeStyles((theme) => ({
  "@global": {
    "em-emoji-picker": {
      "--rgb-background": theme.palette.background.paper,
      "--rgb-input": colors.gray[17],
      "--rgb-color": colors.gray[4],
    },
  },
  adornment: {
    width: theme.spacing(3),
    height: theme.spacing(3),
  },
  iconField: {
    paddingBottom: theme.spacing(0.5),
  },
}))

export default SettingsGroupPageView
