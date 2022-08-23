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
import { getFormHelpersWithError, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"

export const Language = {
  nameLabel: "Name",
  descriptionLabel: "Description",
  maxTtlLabel: "Auto-stop limit",
  iconLabel: "Icon",
  // This is the same from the CLI on https://github.com/coder/coder/blob/546157b63ef9204658acf58cb653aa9936b70c49/cli/templateedit.go#L59
  maxTtlHelperText: "Edit the template maximum time before shutdown in hours",
  formAriaLabel: "Template settings form",
  selectEmoji: "Select emoji",
}

export const validationSchema = Yup.object({
  name: nameValidator(Language.nameLabel),
  description: Yup.string(),
  max_ttl_ms: Yup.number(),
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
  const form: FormikContextType<UpdateTemplateMeta> = useFormik<UpdateTemplateMeta>({
    initialValues: {
      name: template.name,
      description: template.description,
      max_ttl_ms: template.max_ttl_ms,
      icon: template.icon,
    },
    validationSchema,
    onSubmit,
    initialTouched,
  })
  const getFieldHelpers = getFormHelpersWithError<UpdateTemplateMeta>(form, error)
  const styles = useStyles()
  const hasIcon = form.values.icon && form.values.icon !== ""
  const emojiButtonRef = useRef<HTMLButtonElement>(null)

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
                form.setFieldValue("icon", `/emojis/${emojiData.unified}.png`)
                setIsEmojiPickerOpen(false)
              }}
            />
          </Popover>
        </div>

        <TextField
          {...getFieldHelpers("max_ttl_ms")}
          helperText={Language.maxTtlHelperText}
          disabled={isSubmitting}
          fullWidth
          inputProps={{ min: 0, step: 1 }}
          label={Language.maxTtlLabel}
          variant="outlined"
          // Display hours from ms
          value={form.values.max_ttl_ms ? form.values.max_ttl_ms / 3600000 : ""}
          // Convert hours to ms
          onChange={(event) =>
            form.setFieldValue("max_ttl_ms", Number(event.currentTarget.value) * 3600000)
          }
        />
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
