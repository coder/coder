import { ComponentsProps } from "@material-ui/core/styles/props"

/**
 * These are global overrides to MUI components and we may move away from using them in the future.
 */
export const props = {
  MuiButton: {
    variant: "contained",
  },
  MuiTextField: {
    margin: "dense",
    InputProps: {
      labelWidth: 0,
    },
    variant: "outlined",
    spellCheck: false,
  },
  MuiInputLabel: {
    shrink: true,
  },
  MuiFormControl: {
    variant: "outlined",
    margin: "dense",
  },
  MuiInput: {
    spellCheck: false,
    autoCorrect: "off",
  },
  MuiOutlinedInput: {
    notched: false,
  },
  MuiDialogTitle: {
    disableTypography: true,
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
  MuiTab: {
    disableTouchRipple: true,
  },
} as ComponentsProps
