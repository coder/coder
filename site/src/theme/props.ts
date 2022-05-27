import { ComponentsProps } from "@material-ui/core/styles/props"

/**
 * These are global overrides to MUI components and we may move away from using them in the future.
 */
export const props = {
  MuiButtonBase: {
    disableRipple: true,
  },
  MuiButton: {
    variant: "contained",
  },
  MuiTextField: {
    margin: "dense",
    variant: "outlined",
    spellCheck: false,
  },
  MuiFormControl: {
    variant: "outlined",
    margin: "dense",
  },
  MuiMenu: {
    anchorOrigin: {
      vertical: "bottom",
      horizontal: "right",
    },
    transformOrigin: {
      vertical: "top",
      horizontal: "right",
    },
    // Disable the behavior of placing the select at the selected option
    getContentAnchorEl: null,
  },
  MuiList: {
    disablePadding: true,
  },
  MuiTabs: {
    textColor: "primary",
    indicatorColor: "primary",
  },
  MuiPaper: {
    elevation: 0,
  },
} as ComponentsProps
