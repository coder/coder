import { lighten, Theme } from "@material-ui/core/styles"
import { Overrides } from "@material-ui/core/styles/overrides"
import { colors } from "./colors"
import { borderRadius } from "./constants"

export const getOverrides = ({ palette, breakpoints }: Theme): Overrides => {
  return {
    MuiCssBaseline: {
      "@global": {
        body: {
          backgroundImage: `linear-gradient(to right bottom, ${palette.background.default}, ${colors.gray[17]})`,
          backgroundRepeat: "no-repeat",
          backgroundAttachment: "fixed",
        },
        ":root": {
          colorScheme: palette.type,
        },
      },
    },
    MuiAvatar: {
      root: {
        width: 36,
        height: 36,
        fontSize: 18,
      },
      colorDefault: {
        backgroundColor: colors.gray[6],
      },
    },
    MuiButton: {
      root: {
        // Prevents a loading button from collapsing!
        minHeight: 42,
        fontWeight: "normal",
        fontSize: 16,
        textTransform: "none",
        letterSpacing: "none",
        border: `1px solid ${palette.divider}`,

        "&:focus-visible": {
          outline: `2px solid ${palette.primary.dark}`,
        },
      },
      contained: {
        boxShadow: "none",
        color: palette.text.primary,
        backgroundColor: colors.gray[17],

        "&:hover": {
          boxShadow: "none",
          backgroundColor: colors.gray[17],
          borderColor: lighten(palette.divider, 0.2),
        },

        "&.Mui-disabled": {
          backgroundColor: palette.background.paper,
          color: palette.secondary.main,
        },
      },
      sizeSmall: {
        padding: `0 12px`,
        fontSize: 14,
        minHeight: 36,
      },
      iconSizeSmall: {
        width: 16,
        height: 16,
        marginLeft: "0 !important",
        marginRight: 12,
      },
      outlined: {
        border: `1px solid ${palette.divider}`,

        "&:hover": {
          backgroundColor: palette.action.hover,
        },
      },
    },
    MuiIconButton: {
      sizeSmall: {
        "& .MuiSvgIcon-root": {
          width: 20,
          height: 20,
        },
      },
    },
    MuiTableContainer: {
      root: {
        borderRadius,
        border: `1px solid ${palette.divider}`,
      },
    },
    MuiTable: {
      root: {
        borderCollapse: "unset",
        border: "none",
        background: palette.background.paper,
        boxShadow: `0 0 0 1px ${palette.background.default} inset`,
        overflow: "hidden",

        "& td": {
          paddingTop: 16,
          paddingBottom: 16,
          background: "transparent",
        },

        [breakpoints.down("sm")]: {
          // Random value based on visual adjustments.
          // This is used to avoid line breaking on columns
          minWidth: 1000,
        },
      },
    },

    MuiTableCell: {
      head: {
        fontSize: 14,
        color: palette.text.secondary,
        fontWeight: 600,
        background: palette.background.paperLight,
      },
      root: {
        fontSize: 16,
        background: palette.background.paper,
        borderBottom: `1px solid ${palette.divider}`,
        padding: "12px 8px",
        // This targets the first+last td elements, and also the first+last elements
        // of a TableCellLink.
        "&:not(:only-child):first-child, &:not(:only-child):first-child > a": {
          paddingLeft: 32,
        },
        "&:not(:only-child):last-child, &:not(:only-child):last-child > a": {
          paddingRight: 32,
        },
      },
    },

    MuiTableRow: {
      root: {
        "&:last-child .MuiTableCell-body": {
          borderBottom: 0,
        },
      },
    },

    MuiInputBase: {
      root: {
        borderRadius,
      },
    },
    MuiOutlinedInput: {
      root: {
        "& .MuiOutlinedInput-notchedOutline": {
          borderColor: palette.divider,
        },

        "& input:-webkit-autofill": {
          WebkitBoxShadow: `0 0 0 1000px ${palette.background.paper} inset`,
        },

        "&:hover:not(.Mui-focused) .MuiOutlinedInput-notchedOutline": {
          borderColor: palette.divider,
        },
      },
    },
    MuiLink: {
      root: {
        color: palette.primary.light,
      },
    },
    MuiPaper: {
      root: {
        borderRadius,
        border: `1px solid ${palette.divider}`,
      },
    },
    MuiFormHelperText: {
      contained: {
        marginLeft: 0,
        marginRight: 0,
      },

      marginDense: {
        marginTop: 8,
      },
    },
  }
}
