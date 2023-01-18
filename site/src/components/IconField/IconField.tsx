import Button from "@material-ui/core/Button"
import InputAdornment from "@material-ui/core/InputAdornment"
import Popover from "@material-ui/core/Popover"
import TextField from "@material-ui/core/TextField"
import { OpenDropdown } from "components/DropdownArrows/DropdownArrows"
import { useRef, FC, useState } from "react"
import Picker from "@emoji-mart/react"
import { makeStyles } from "@material-ui/core/styles"
import { colors } from "theme/colors"
import { useTranslation } from "react-i18next"
import data from "@emoji-mart/data/sets/14/twitter.json"
import { IconFieldProps } from "./types"

const IconField: FC<IconFieldProps> = ({ onPickEmoji, ...textFieldProps }) => {
  if (
    typeof textFieldProps.value !== "string" &&
    typeof textFieldProps.value !== "undefined"
  ) {
    throw new Error(`Invalid icon value "${typeof textFieldProps.value}"`)
  }

  const styles = useStyles()
  const emojiButtonRef = useRef<HTMLButtonElement>(null)
  const [isEmojiPickerOpen, setIsEmojiPickerOpen] = useState(false)
  const { t } = useTranslation("templateSettingsPage")
  const hasIcon = textFieldProps.value && textFieldProps.value !== ""

  return (
    <div className={styles.iconField}>
      <TextField
        {...textFieldProps}
        fullWidth
        label={t("iconLabel")}
        variant="outlined"
        InputProps={{
          endAdornment: hasIcon ? (
            <InputAdornment position="end" className={styles.adornment}>
              <img
                alt=""
                src={textFieldProps.value}
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
        {t("selectEmoji")}
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
            const value = `/emojis/${emojiData.unified.replace(
              /-fe0f$/,
              "",
            )}.png`
            onPickEmoji(value)
            setIsEmojiPickerOpen(false)
          }}
        />
      </Popover>
    </div>
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
    display: "flex",
    alignItems: "center",
    justifyContent: "center",

    "& img": {
      maxWidth: "100%",
    },
  },
  iconField: {
    paddingBottom: theme.spacing(0.5),
  },
}))

export default IconField
