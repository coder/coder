import Box from "@material-ui/core/Box"
import Checkbox from "@material-ui/core/Checkbox"
import Typography from "@material-ui/core/Typography"
import data from "@emoji-mart/data/sets/14/twitter.json"
import Picker from "@emoji-mart/react"
import Button from "@material-ui/core/Button"
import InputAdornment from "@material-ui/core/InputAdornment"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { OpenDropdown } from "components/DropdownArrows/DropdownArrows"
import { FormFooter } from "components/FormFooter/FormFooter"
import { Stack } from "components/Stack/Stack"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC, useRef, useState } from "react"
import { colors } from "theme/colors"
import {
  getFormHelpers,
  nameValidator,
  templateDisplayNameValidator,
  onChangeTrimmed,
} from "util/formUtils"
import * as Yup from "yup"
import i18next from "i18next"
import { useTranslation } from "react-i18next"

export const Language = {
  nameLabel: "Name",
  descriptionLabel: "Description",
  defaultTtlLabel: "Auto-stop default",
  iconLabel: "Icon",
  formAriaLabel: "Template settings form",
  selectEmoji: "Select emoji",
  ttlMaxError:
    "Please enter a limit that is less than or equal to 168 hours (7 days).",
  descriptionMaxError:
    "Please enter a description that is less than or equal to 128 characters.",
  ttlHelperText: (ttl: number): string =>
    `Workspaces created from this template will default to stopping after ${ttl} hours.`,
}

const MAX_DESCRIPTION_CHAR_LIMIT = 128
const MAX_TTL_DAYS = 7
const MS_HOUR_CONVERSION = 3600000

export const validationSchema = Yup.object({
  name: nameValidator(Language.nameLabel),
  display_name: templateDisplayNameValidator(
    i18next.t("displayNameLabel", {
      ns: "templatePage",
    }),
  ),
  description: Yup.string().max(
    MAX_DESCRIPTION_CHAR_LIMIT,
    Language.descriptionMaxError,
  ),
  default_ttl_ms: Yup.number()
    .integer()
    .min(0)
    .max(24 * MAX_TTL_DAYS /* 7 days in hours */, Language.ttlMaxError),
  allow_user_cancel_workspace_jobs: Yup.boolean(),
})

export interface TemplateSettingsForm {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  error?: unknown
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>
}

export const TemplateSettingsForm: FC<TemplateSettingsForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  isSubmitting,
  initialTouched,
}) => {
  const [isEmojiPickerOpen, setIsEmojiPickerOpen] = useState(false)
  const form: FormikContextType<UpdateTemplateMeta> =
    useFormik<UpdateTemplateMeta>({
      initialValues: {
        name: template.name,
        display_name: template.display_name,
        description: template.description,
        // on display, convert from ms => hours
        default_ttl_ms: template.default_ttl_ms / MS_HOUR_CONVERSION,
        icon: template.icon,
        allow_user_cancel_workspace_jobs:
          template.allow_user_cancel_workspace_jobs,
      },
      validationSchema,
      onSubmit: (formData) => {
        // on submit, convert from hours => ms
        onSubmit({
          ...formData,
          default_ttl_ms: formData.default_ttl_ms
            ? formData.default_ttl_ms * MS_HOUR_CONVERSION
            : undefined,
        })
      },
      initialTouched,
    })
  const getFieldHelpers = getFormHelpers<UpdateTemplateMeta>(form, error)
  const styles = useStyles()
  const hasIcon = form.values.icon && form.values.icon !== ""
  const emojiButtonRef = useRef<HTMLButtonElement>(null)

  const { t } = useTranslation("templatePage")

  return (
    <form onSubmit={form.handleSubmit} aria-label={Language.formAriaLabel}>
      <Stack>
        <TextField
          {...getFieldHelpers("name")}
          disabled={isSubmitting}
          onChange={onChangeTrimmed(form)}
          autoFocus
          fullWidth
          label={Language.nameLabel}
          variant="outlined"
        />

        <TextField
          {...getFieldHelpers("display_name")}
          disabled={isSubmitting}
          fullWidth
          label={t("displayNameLabel")}
          variant="outlined"
        />

        <TextField
          {...getFieldHelpers("description")}
          multiline
          disabled={isSubmitting}
          fullWidth
          label={Language.descriptionLabel}
          variant="outlined"
          rows={2}
        />

        <div className={styles.iconField}>
          <TextField
            {...getFieldHelpers("icon")}
            disabled={isSubmitting}
            fullWidth
            label={Language.iconLabel}
            variant="outlined"
            InputProps={{
              endAdornment: hasIcon ? (
                <InputAdornment position="end">
                  <img
                    alt=""
                    src={form.values.icon}
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
            {Language.selectEmoji}
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
                // See: https://github.com/missive/emoji-mart/issues/51#issuecomment-287353222
                form.setFieldValue(
                  "icon",
                  `/emojis/${emojiData.unified.replace(/-fe0f$/, "")}.png`,
                )
                setIsEmojiPickerOpen(false)
              }}
            />
          </Popover>
        </div>

        <TextField
          {...getFieldHelpers("default_ttl_ms")}
          disabled={isSubmitting}
          fullWidth
          inputProps={{ min: 0, step: 1 }}
          label={Language.defaultTtlLabel}
          variant="outlined"
          type="number"
        />
        {/* If a value for default_ttl_ms has been entered and
        there are no validation errors for that field, display helper text.
        We do not use the MUI helper-text prop because it overrides the validation error */}
        {form.values.default_ttl_ms && !form.errors.default_ttl_ms && (
          <span>{Language.ttlHelperText(form.values.default_ttl_ms)}</span>
        )}

        <Box display="flex">
          <div>
            {/*"getFieldHelpers" can't be used as it requires "helperText" property to be present.*/}
            <Checkbox
              id="allow_user_cancel_workspace_jobs"
              name="allow_user_cancel_workspace_jobs"
              disabled={isSubmitting}
              checked={form.values.allow_user_cancel_workspace_jobs}
              onChange={form.handleChange}
            />
          </div>
          <Box>
            <Typography variant="h6" style={{ fontSize: 14 }}>
              {t("allowUserCancelWorkspaceJobsLabel")}
            </Typography>
            <Typography variant="caption" color="textSecondary">
              {t("allowUserCancelWorkspaceJobsNotice")}
            </Typography>
          </Box>
        </Box>
      </Stack>

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </form>
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
