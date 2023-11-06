import { css, Global, useTheme } from "@emotion/react";
import Button from "@mui/material/Button";
import InputAdornment from "@mui/material/InputAdornment";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import Picker from "@emoji-mart/react";
import { type FC } from "react";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Stack } from "components/Stack/Stack";
import { colors } from "theme/colors";
import data from "@emoji-mart/data/sets/14/twitter.json";
import icons from "theme/icons.json";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";

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

  const theme = useTheme();
  const hasIcon = textFieldProps.value && textFieldProps.value !== "";

  return (
    <Stack spacing={1}>
      <TextField
        {...textFieldProps}
        fullWidth
        label="Icon"
        InputProps={{
          endAdornment: hasIcon ? (
            <InputAdornment
              position="end"
              css={{
                width: theme.spacing(3),
                height: theme.spacing(3),
                display: "flex",
                alignItems: "center",
                justifyContent: "center",

                "& img": {
                  maxWidth: "100%",
                  objectFit: "contain",
                },
              }}
            >
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

      <Popover>
        {(popover) => (
          <>
            <PopoverTrigger>
              <Button fullWidth endIcon={<DropdownArrow />}>
                Select emoji
              </Button>
            </PopoverTrigger>
            <PopoverContent
              id="emoji"
              css={{ marginTop: 0, ".MuiPaper-root": { width: "auto" } }}
            >
              <Global
                styles={css`
                  em-emoji-picker {
                    --rgb-background: ${theme.palette.background.paper};
                    --rgb-input: ${colors.gray[17]};
                    --rgb-color: ${colors.gray[4]};

                    // Hack to prevent the right side from being cut off
                    width: 350px;
                  }
                `}
              />
              <Picker
                set="twitter"
                theme="dark"
                data={data}
                custom={custom}
                onEmojiSelect={(emoji) => {
                  const value = emoji.src ?? urlFromUnifiedCode(emoji.unified);
                  onPickEmoji(value);
                  popover.setIsOpen(false);
                }}
              />
            </PopoverContent>
          </>
        )}
      </Popover>
    </Stack>
  );
};

export default IconField;
