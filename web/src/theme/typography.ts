import { TypographyOptions } from "@material-ui/core/styles/createTypography"
import { BODY_FONT_FAMILY } from "./constants"

export const typography: TypographyOptions = {
  fontFamily: BODY_FONT_FAMILY,
  fontSize: 16,
  fontWeightLight: 300,
  fontWeightRegular: 400,
  fontWeightMedium: 500,
  h1: {
    fontSize: 72,
    fontWeight: 400,
    letterSpacing: -2,
  },
  h2: {
    fontSize: 64,
    letterSpacing: -2,
    fontWeight: 400,
  },
  h3: {
    fontSize: 32,
    letterSpacing: -0.3,
    fontWeight: 400,
  },
  h4: {
    fontSize: 24,
    letterSpacing: -0.3,
    fontWeight: 400,
  },
  h5: {
    fontSize: 20,
    letterSpacing: -0.3,
    fontWeight: 400,
  },
  h6: {
    fontSize: 16,
    fontWeight: 600,
  },
  body1: {
    fontSize: 16,
    lineHeight: "24px",
  },
  body2: {
    fontSize: 14,
    lineHeight: "20px",
  },
  subtitle1: {
    fontSize: 18,
    lineHeight: "28px",
  },
  subtitle2: {
    fontSize: 16,
    lineHeight: "24px",
  },
  caption: {
    fontSize: 12,
    lineHeight: "16px",
  },
  overline: {
    fontSize: 12,
    fontWeight: 500,
    lineHeight: "16px",
    letterSpacing: 1.5,
    textTransform: "uppercase",
  },
  button: {
    fontSize: 13,
    fontWeight: 600,
    textTransform: "uppercase",
    letterSpacing: 1.5,
  },
}
