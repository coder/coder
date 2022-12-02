import { makeStyles } from "@material-ui/core/styles"
import ReactMarkdown from "react-markdown"
import { colors } from "theme/colors"

export interface ServiceBannerViewProps {
  message: string
  backgroundColor: string
}

export const ServiceBannerView: React.FC<ServiceBannerViewProps> = ({
  message,
  backgroundColor,
}) => {
  const styles = useStyles()
  const markdownElementsAllowed = [
    "text",
    "a",
    "strong",
    "delete",
    "emphasis",
    "link",
  ]
  return (
    <div
      className={`${styles.container}`}
      style={{ backgroundColor: backgroundColor }}
    >
      <div className={styles.centerContent}>
        <ReactMarkdown
          allowedElements={markdownElementsAllowed}
          linkTarget="_blank"
          unwrapDisallowed
        >
          {message}
        </ReactMarkdown>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  container: {
    padding: theme.spacing(1.5),
    backgroundColor: theme.palette.warning.main,
    display: "flex",
    alignItems: "center",

    "&.error": {
      backgroundColor: colors.red[12],
    },
  },
  flex: {
    display: "column",
  },
  centerContent: {
    marginRight: "auto",
    marginLeft: "auto",
    // Automatically pick high-contrast foreground text.
    // "difference" is the most correct way of implementing this
    // but "exclusion" looks prettier for most colors.
    mixBlendMode: "exclusion",
  },
  link: {
    color: "inherit",
    textDecoration: "none",
    fontWeight: "bold",
  },
  list: {
    padding: theme.spacing(1),
    margin: 0,
  },
  listItem: {
    margin: theme.spacing(0.5),
  },
}))
