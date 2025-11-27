/**
 * @deprecated MUI theme configuration is deprecated. Migrate to Tailwind CSS theme system.
 * This file provides MUI theme overrides for legacy compatibility only.
 */

/** @deprecated MUI Alert classes are deprecated. Use shadcn/ui Alert component instead. */
// biome-ignore lint/style/noRestrictedImports: we use the classes for customization
import { alertClasses } from "@mui/material/Alert";
/** @deprecated MUI ThemeOptions is deprecated. Migrate to Tailwind CSS theme system. */
// biome-ignore lint/style/noRestrictedImports: we use the MUI theme as a base
import type { ThemeOptions } from "@mui/material/styles";
import {
	BODY_FONT_FAMILY,
	BUTTON_LG_HEIGHT,
	BUTTON_MD_HEIGHT,
} from "./constants";
import tw from "./tailwindColors";

// biome-ignore lint/suspicious/noExplicitAny: needed for MUI overrides
type MuiStyle = any;

export const components = {
	MuiCssBaseline: {
		styleOverrides: (theme) => `
      html, body, #root, #storybook-root {
        height: 100%;
      }

      button, input {
        font-family: ${BODY_FONT_FAMILY};
      }

      input:-webkit-autofill,
      input:-webkit-autofill:hover,
      input:-webkit-autofill:focus,
      input:-webkit-autofill:active  {
        -webkit-box-shadow: 0 0 0 100px ${theme.palette.background.default} inset !important;
      }

      ::placeholder {
        color: ${theme.palette.text.disabled};
      }

      fieldset {
        border: unset;
        padding: 0;
        margin: 0;
        width: 100%;
      }
    `,
	},
	MuiAvatar: {
		styleOverrides: {
			root: {
				width: 36,
				height: 36,
				fontSize: 18,

				"& .MuiSvgIcon-root": {
					width: "50%",
				},
			},
			colorDefault: ({ theme }) => ({
				backgroundColor: theme.palette.primary.light,
			}),
		},
	},
	MuiLink: {
		defaultProps: {
			underline: "hover",
		},
	},
	MuiPaper: {
		defaultProps: {
			elevation: 0,
		},
		styleOverrides: {
			root: ({ theme }) => ({
				border: `1px solid ${theme.palette.divider}`,
				backgroundImage: "none",
			}),
		},
	},
	MuiSkeleton: {
		styleOverrides: {
			root: ({ theme }) => ({
				backgroundColor: theme.palette.divider,
			}),
		},
	},
	MuiLinearProgress: {
		styleOverrides: {
			root: {
				borderRadius: 999,
			},
		},
	},
	MuiChip: {
		styleOverrides: {
			root: {
				backgroundColor: tw.zinc[600],
			},
		},
	},
	MuiMenu: {
		defaultProps: {
			anchorOrigin: {
				vertical: "bottom",
				horizontal: "right",
			},
			transformOrigin: {
				vertical: "top",
				horizontal: "right",
			},
		},
		styleOverrides: {
			paper: {
				marginTop: 8,
				borderRadius: 4,
				padding: "4px 0",
				minWidth: 160,
			},
			root: {
				// It should be the same as the menu padding
				"& .MuiDivider-root": {
					marginTop: "4px !important",
					marginBottom: "4px !important",
				},
			},
		},
	},
	MuiMenuItem: {
		styleOverrides: {
			root: {
				gap: 12,

				"& .MuiSvgIcon-root": {
					fontSize: 20,
				},
			},
		},
	},
	MuiSnackbar: {
		styleOverrides: {
			anchorOriginBottomRight: {
				bottom: `${24 + 36}px !important`, // 36 is the bottom bar height
			},
		},
	},
	MuiSnackbarContent: {
		styleOverrides: {
			root: {
				borderRadius: "4px !important",
			},
		},
	},
	MuiTextField: {
		defaultProps: {
			InputLabelProps: {
				shrink: true,
			},
		},
	},
	MuiInputBase: {
		defaultProps: {
			color: "primary",
		},
		styleOverrides: {
			root: {
				height: BUTTON_LG_HEIGHT,
			},
			sizeSmall: {
				height: BUTTON_MD_HEIGHT,
				fontSize: 14,
			},
			multiline: {
				height: "auto",
			},
			["colorPrimary" as MuiStyle]: {
				// Same as button
				"& .MuiOutlinedInput-notchedOutline": {
					borderColor: tw.zinc[600],
				},
				// The default outlined input color is white, which seemed jarring.
				"&:hover:not(.Mui-error):not(.Mui-focused) .MuiOutlinedInput-notchedOutline":
					{
						borderColor: tw.zinc[500],
					},
			},
		},
	},
	MuiFormHelperText: {
		defaultProps: {
			sx: {
				marginLeft: 0,
				marginTop: 1,
				"&::first-letter": {
					// Server errors are returned in all lowercase. To display them as
					// field errors in the UI, we capitalize the first letter.
					textTransform: "uppercase",
				},
			},
		},
	},
	MuiRadio: {
		defaultProps: {
			disableRipple: true,
		},
	},
	MuiCheckbox: {
		styleOverrides: {
			root: {
				/**
				 * Adds focus styling to checkboxes (which doesn't exist normally, for
				 * some reason?).
				 *
				 * The checkbox component is a root span with a checkbox input inside
				 * it. MUI does not allow you to use selectors like (& input) to
				 * target the inner checkbox (even though you can use & td to style
				 * tables). Tried every combination of selector possible (including
				 * lots of !important), and the main issue seems to be that the
				 * styling just never gets processed for it to get injected into the
				 * CSSOM.
				 *
				 * Had to settle for adding styling to the span itself (which does
				 * make the styling more obvious, even if there's not much room for
				 * customization).
				 */
				"&.Mui-focusVisible": {
					boxShadow: `0 0 0 2px ${tw.blue[400]}`,
				},

				"&.Mui-disabled": {
					color: tw.zinc[500],
				},

				"& .MuiSvgIcon-fontSizeXsmall": {
					fontSize: "1rem",
				},
			},
		},
	},
	MuiSwitch: {
		defaultProps: { color: "primary" },
		styleOverrides: {
			root: {
				".Mui-focusVisible .MuiSwitch-thumb": {
					// Had to thicken outline to make sure that the focus color didn't
					// bleed into the thumb and was still easily-visible
					boxShadow: `0 0 0 3px ${tw.blue[400]}`,
				},
			},
		},
	},
	MuiAutocomplete: {
		styleOverrides: {
			root: {
				// Not sure why but since the input has padding we don't need it here
				"& .MuiInputBase-root": {
					padding: "0px 0px 0px 8px",
				},
			},
		},
	},
	MuiList: {
		defaultProps: {
			disablePadding: true,
		},
	},
	MuiTabs: {
		defaultProps: {
			textColor: "primary",
			indicatorColor: "primary",
		},
	},
	MuiTooltip: {
		styleOverrides: {
			tooltip: ({ theme }) => ({
				lineHeight: "150%",
				borderRadius: 4,
				background: theme.palette.divider,
				padding: "8px 16px",
			}),
			arrow: ({ theme }) => ({
				color: theme.palette.divider,
			}),
		},
	},
	MuiAlert: {
		defaultProps: {
			variant: "outlined",
		},
		styleOverrides: {
			root: ({ theme }) => ({
				background: theme.palette.background.paper,
			}),
			action: {
				paddingTop: 2, // Idk why it is not aligned as expected
			},
			icon: {
				fontSize: 16,
				marginTop: "4px", // The size of text is 24 so (24 - 16)/2 = 4
			},
			message: ({ theme }) => ({
				color: theme.palette.text.primary,
			}),
			outlinedWarning: ({ theme }) => ({
				[`& .${alertClasses.icon}`]: {
					color: theme.palette.warning.light,
				},
			}),
			outlinedInfo: ({ theme }) => ({
				[`& .${alertClasses.icon}`]: {
					color: theme.palette.primary.light,
				},
			}),
			outlinedError: ({ theme }) => ({
				[`& .${alertClasses.icon}`]: {
					color: theme.palette.error.light,
				},
			}),
		},
	},
	MuiAlertTitle: {
		styleOverrides: {
			root: {
				fontSize: "inherit",
				marginBottom: 0,
			},
		},
	},
} satisfies ThemeOptions["components"];
