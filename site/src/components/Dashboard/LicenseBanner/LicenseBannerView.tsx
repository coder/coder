import Link from "@mui/material/Link";
import { makeStyles } from "@mui/styles";
import { Expander } from "components/Expander/Expander";
import { Pill } from "components/Pill/Pill";
import { useState } from "react";
import { colors } from "theme/colors";

export const Language = {
  licenseIssue: "License Issue",
  licenseIssues: (num: number): string => `${num} License Issues`,
  upgrade: "Contact sales@coder.com.",
  exceeded: "It looks like you've exceeded some limits of your license.",
  lessDetails: "Less",
  moreDetails: "More",
};

export interface LicenseBannerViewProps {
  errors: string[];
  warnings: string[];
}

export const LicenseBannerView: React.FC<LicenseBannerViewProps> = ({
  errors,
  warnings,
}) => {
  const styles = useStyles();
  const [showDetails, setShowDetails] = useState(false);
  const isError = errors.length > 0;
  const messages = [...errors, ...warnings];
  const type = isError ? "error" : "warning";

  if (messages.length === 1) {
    return (
      <div className={`${styles.container} ${type}`}>
        <Pill text={Language.licenseIssue} type={type} lightBorder />
        <div className={styles.leftContent}>
          <span>{messages[0]}</span>
          &nbsp;
          <Link color="white" fontWeight="medium" href="mailto:sales@coder.com">
            {Language.upgrade}
          </Link>
        </div>
      </div>
    );
  } else {
    return (
      <div className={`${styles.container} ${type}`}>
        <Pill
          text={Language.licenseIssues(messages.length)}
          type={type}
          lightBorder
        />
        <div className={styles.leftContent}>
          <div>
            {Language.exceeded}
            &nbsp;
            <Link
              color="white"
              fontWeight="medium"
              href="mailto:sales@coder.com"
            >
              {Language.upgrade}
            </Link>
          </div>
          <Expander expanded={showDetails} setExpanded={setShowDetails}>
            <ul className={styles.list}>
              {messages.map((message) => (
                <li className={styles.listItem} key={message}>
                  {message}
                </li>
              ))}
            </ul>
          </Expander>
        </div>
      </div>
    );
  }
};

const useStyles = makeStyles((theme) => ({
  container: {
    ...theme.typography.body2,
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
  leftContent: {
    marginRight: theme.spacing(1),
    marginLeft: theme.spacing(1),
  },
  list: {
    padding: theme.spacing(1),
    margin: 0,
  },
  listItem: {
    margin: theme.spacing(0.5),
  },
}));
