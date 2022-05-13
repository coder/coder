import { emptyBoxShadow, lightButtonShadow } from "./constants"
import { CustomPalette } from "./palettes"

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
export const getOverrides = (palette: CustomPalette) => {
  return {
    MuiAvatar: {
      root: {
        width: 32,
        height: 32,
        fontSize: 24,
      },
    },
    // MuiButton: {
    //   root: {
    //     minHeight: 40,
    //     paddingLeft: 20,
    //     paddingRight: 20,
    //     fontWeight: 500,

    //     "& .MuiSvgIcon-root": {
    //       verticalAlign: "middle",
    //     },
    //   },

    //   contained: {
    //     backgroundColor: palette.hero.button,
    //     color: palette.primary.contrastText,
    //     boxShadow: containedButtonShadow,
    //     "&:hover": {
    //       backgroundColor: darken(palette.hero.button, 0.25),
    //     },
    //     "&$disabled": {
    //       color: fade(palette.text.disabled, 0.5),
    //     },
    //   },
    //   containedPrimary: {
    //     boxShadow: containedButtonShadow,
    //   },
    //   containedSecondary: {
    //     boxShadow: containedButtonShadow,
    //   },

    //   outlined: {
    //     borderColor: palette.action.disabled,
    //     borderWidth: buttonBorderWidth,
    //     "&:hover": {
    //       color: palette.primary.main,
    //       borderColor: palette.primary.main,
    //       borderWidth: buttonBorderWidth,
    //     },
    //     "&$disabled": {
    //       color: fade(palette.text.disabled, 0.5),
    //     },
    //   },
    //   outlinedPrimary: {
    //     borderColor: palette.primary.main,
    //     borderWidth: buttonBorderWidth,
    //     "&:hover": {
    //       borderWidth: buttonBorderWidth,
    //     },
    //   },
    //   outlinedSecondary: {
    //     borderColor: palette.secondary.main,
    //     borderWidth: buttonBorderWidth,
    //     "&:hover": {
    //       color: palette.secondary.main,
    //       borderWidth: buttonBorderWidth,
    //     },
    //   },

    //   text: {
    //     "&$disabled": {
    //       color: fade(palette.text.disabled, 0.5),
    //     },
    //   },

    //   sizeSmall: {
    //     minHeight: 32,
    //     paddingLeft: 10,
    //     paddingRight: 10,
    //     letterSpacing: 1.2,
    //     fontSize: 13,

    //     "&.MuiButton-outlined": {
    //       borderWidth: 1,
    //     },
    //   },
    //   sizeLarge: {
    //     minHeight: 46,
    //     paddingLeft: spacing * 3,
    //     paddingRight: spacing * 3,
    //   },
    // },
  }
}
