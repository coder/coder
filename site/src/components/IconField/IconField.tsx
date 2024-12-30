import { Global, css, useTheme } from "@emotion/react";
import Button from "@mui/material/Button";
import InputAdornment from "@mui/material/InputAdornment";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import { visuallyHidden } from "@mui/utils";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import { type FC, Suspense, lazy, useState } from "react";

// See: https://github.com/missive/emoji-mart/issues/51#issuecomment-287353222
const urlFromUnifiedCode = (unified: string) =>
	`/emojis/${unified.replace(/-fe0f$/, "")}.png`;

type IconFieldProps = TextFieldProps & {
	onPickEmoji: (value: string) => void;
};

const EmojiPicker = lazy(() => import("./EmojiPicker"));

export const IconField: FC<IconFieldProps> = ({
	onPickEmoji,
	...textFieldProps
}) => {
	if (
		typeof textFieldProps.value !== "string" &&
		typeof textFieldProps.value !== "undefined"
	) {
		throw new Error(`Invalid icon value "${typeof textFieldProps.value}"`);
	}

	const theme = useTheme();
	const hasIcon = textFieldProps.value && textFieldProps.value !== "";
	const [open, setOpen] = useState(false);

	return (
		<Stack spacing={1}>
			<TextField
				fullWidth
				label="Icon"
				{...textFieldProps}
				InputProps={{
					endAdornment: hasIcon ? (
						<InputAdornment
							position="end"
							css={{
								width: 24,
								height: 24,
								display: "flex",
								alignItems: "center",
								justifyContent: "center",

								"& img": {
									maxWidth: "100%",
									objectFit: "contain",
								},
							}}
						>
							<ExternalImage
								alt=""
								src={textFieldProps.value}
								// This prevent browser to display the ugly error icon if the
								// image path is wrong or user didn't finish typing the url
								onError={(e) => {
									e.currentTarget.style.display = "none";
								}}
								onLoad={(e) => {
									e.currentTarget.style.display = "inline";
								}}
							/>
						</InputAdornment>
					) : undefined,
				}}
			/>

			<Global
				styles={css`
          em-emoji-picker {
            --rgb-background: ${theme.palette.background.paper};
            --rgb-input: ${theme.palette.primary.main};
            --rgb-color: ${theme.palette.text.primary};

            // Hack to prevent the right side from being cut off
            width: 350px;
          }
        `}
			/>
			<Popover open={open} onOpenChange={setOpen}>
				<PopoverTrigger>
					<Button fullWidth endIcon={<DropdownArrow />}>
						Select emoji
					</Button>
				</PopoverTrigger>
				<PopoverContent
					id="emoji"
					css={{ marginTop: 0, ".MuiPaper-root": { width: "auto" } }}
				>
					<Suspense fallback={<Loader />}>
						<EmojiPicker
							onEmojiSelect={(emoji) => {
								const value = emoji.src ?? urlFromUnifiedCode(emoji.unified);
								onPickEmoji(value);
								setOpen(false);
							}}
						/>
					</Suspense>
				</PopoverContent>
			</Popover>

			{/*
      - This component takes a long time to load (easily several seconds), so we
      don't want to wait until the user actually clicks the button to start loading.
      Unfortunately, React doesn't provide an API to start warming a lazy component,
      so we just have to sneak it into the DOM, which is kind of annoying, but means
      that users shouldn't ever spend time waiting for it to load.
      - Except we don't do it when running tests, because Jest doesn't define
      `IntersectionObserver`, and it would make them slower anyway. */}
			{process.env.NODE_ENV !== "test" && (
				<div css={{ ...visuallyHidden }}>
					<Suspense>
						<EmojiPicker onEmojiSelect={() => {}} />
					</Suspense>
				</div>
			)}
		</Stack>
	);
};
