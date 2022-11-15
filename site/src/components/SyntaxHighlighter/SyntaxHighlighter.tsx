import { makeStyles } from "@material-ui/core/styles"
import { ComponentProps, FC } from "react"
import { Prism } from "react-syntax-highlighter"
import { colors } from "theme/colors"
import darcula from "react-syntax-highlighter/dist/cjs/styles/prism/darcula"
import { combineClasses } from "util/combineClasses"

export const SyntaxHighlighter: FC<ComponentProps<typeof Prism>> = ({
  className,
  ...props
}) => {
  const styles = useStyles()

  return (
    <Prism
      style={darcula}
      useInlineStyles={false}
      // Use inline styles does not work correctly
      // https://github.com/react-syntax-highlighter/react-syntax-highlighter/issues/329
      codeTagProps={{ style: {} }}
      className={combineClasses([styles.prism, className])}
      {...props}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  prism: {
    margin: 0,
    background: theme.palette.background.paperLight,
    borderRadius: theme.shape.borderRadius,
    padding: theme.spacing(2, 3),
    // Line breaks are broken when used with line numbers on react-syntax-highlighter
    // https://github.com/react-syntax-highlighter/react-syntax-highlighter/pull/483
    overflowX: "auto",

    "& code": {
      color: theme.palette.text.secondary,
    },

    "& .key, & .property, & .code-snippet, & .keyword": {
      color: colors.turquoise[7],
    },

    "& .url": {
      color: colors.blue[6],
    },

    "& .comment": {
      color: theme.palette.text.disabled,
    },

    "& .title": {
      color: theme.palette.text.primary,
      fontWeight: 600,
    },
  },
}))
