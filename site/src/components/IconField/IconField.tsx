import Button from "@mui/material/Button";
import InputAdornment from "@mui/material/InputAdornment";
import Popover from "@mui/material/Popover";
import TextField, { TextFieldProps } from "@mui/material/TextField";
import { makeStyles } from "@mui/styles";
import Picker from "@emoji-mart/react";
import { useRef, FC, useState } from "react";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Stack } from "components/Stack/Stack";
import { colors } from "theme/colors";
import data from "@emoji-mart/data/sets/14/twitter.json";
import icons from "theme/icons.json";

// See: https://github.com/missive/emoji-mart/issues/51#issuecomment-287353222
const urlFromUnifiedCode = (unified: string) =>
  `/emojis/${unified.replace(/-fe0f$/, "")}.png`;

type IconFieldProps = TextFieldProps & {
  onPickEmoji: (value: string) => void;
};

const custom = [
  {
    id: "icons",
    name: "Icons",
    emojis: icons.map((icon) => {
      const id = icon.split(".")[0];

      return {
        id,
        name: id,
        keywords: id.split("-"),
        skins: [{ src: `/icon/${icon}` }],
      };
    }),
  },
];

const IconField: FC<IconFieldProps> = ({ onPickEmoji, ...textFieldProps }) => {
  if (
    typeof textFieldProps.value !== "string" &&
    typeof textFieldProps.value !== "undefined"
  ) {
    throw new Error(`Invalid icon value "${typeof textFieldProps.value}"`);
  }

  const styles = useStyles();
  const emojiButtonRef = useRef<HTMLButtonElement>(null);
  const [isEmojiPickerOpen, setIsEmojiPickerOpen] = useState(false);
  const hasIcon = textFieldProps.value && textFieldProps.value !== "";

  return (
    <Stack spacing={1}>
      <TextField
        {...textFieldProps}
        fullWidth
        label="Icon"
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
        endIcon={<DropdownArrow />}
        onClick={() => {
          setIsEmojiPickerOpen((v) => !v);
        }}
      >
        Select emoji
      </Button>

      <Popover
        id="emoji"
        open={isEmojiPickerOpen}
        anchorEl={emojiButtonRef.current}
        onClose={() => {
          setIsEmojiPickerOpen(false);
        }}
      >
        <Picker
          set="twitter"
          theme="dark"
          data={data}
          custom={custom}
          onEmojiSelect={(emoji) => {
            const value = emoji.src ?? urlFromUnifiedCode(emoji.unified);
            onPickEmoji(value);
            setIsEmojiPickerOpen(false);
          }}
        />
      </Popover>
    </Stack>
  );
};

const useStyles = makeStyles((theme) => ({
  "@global": {
    "em-emoji-picker": {
      "--rgb-background": theme.palette.background.paper,
      "--rgb-input": colors.gray[17],
      "--rgb-color": colors.gray[4],

      // Hack to prevent the right side from being cut off
      width: 350,
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
      objectFit: "contain",
    },
  },
}));

export default IconField;
