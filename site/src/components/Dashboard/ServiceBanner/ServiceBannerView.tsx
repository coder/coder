import { makeStyles } from "@mui/styles";
import { Pill } from "components/Pill/Pill";
import ReactMarkdown from "react-markdown";
import { colors } from "theme/colors";
import { hex } from "color-convert";

export interface ServiceBannerViewProps {
  message: string;
  backgroundColor: string;
  preview: boolean;
}

export const ServiceBannerView: React.FC<ServiceBannerViewProps> = ({
  message,
  backgroundColor,
  preview,
}) => {
  const styles = useStyles();
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
  ];
  return (
    <div
      className={styles.container}
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
  );
};

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
}));

const readableForegroundColor = (backgroundColor: string): string => {
  const rgb = hex.rgb(backgroundColor);

  // Logic taken from here:
  // https://github.com/casesandberg/react-color/blob/bc9a0e1dc5d11b06c511a8e02a95bd85c7129f4b/src/helpers/color.js#L56
  // to be consistent with the color-picker label.
  const yiq = (rgb[0] * 299 + rgb[1] * 587 + rgb[2] * 114) / 1000;
  return yiq >= 128 ? "#000" : "#fff";
};
