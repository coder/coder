import { makeStyles } from "@material-ui/core/styles"
import { Pill } from "components/Pill/Pill"
import ReactMarkdown from "react-markdown"
import { colors } from "theme/colors"
import { hex } from "color-convert"

export interface ServiceBannerViewProps {
  message: string
  backgroundColor: string
  preview: boolean
}

export const ServiceBannerView: React.FC<ServiceBannerViewProps> = ({
  message,
  backgroundColor,
  preview,
}) => {
  const styles = useStyles()
  // We don't want anything funky like an image or a heading in the service
  // banner.
  const markdownElementsAllowed = [
    "text",
    "a",
    "pre",
    "ul",
    "strong",
    "emphasis",
    "italic",
    "link",
    "em",
  ]
  return (
    <div
      className={`${styles.container}`}
      style={{ backgroundColor: backgroundColor }}
    >
      {preview && <Pill text="Preview" type="info" lightBorder />}
      <div
        className={styles.centerContent}
        style={{
          color: readableForegroundColor(backgroundColor),
        }}
      >
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
    fontWeight: 400,
    "& a": {
      color: "inherit",
    },
  },
}))

const readableForegroundColor = (backgroundColor: string): string => {
  const [_, __, lum] = hex.hsl(backgroundColor)
  if (lum > 50) {
    return "black"
  }
  return "white"
}
