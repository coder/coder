import {
  type CSSObject,
  type Interpolation,
  type Theme,
  useTheme,
} from "@emotion/react";
import Link from "@mui/material/Link";
import { css } from "@emotion/react";
import { useState } from "react";
import { Expander } from "components/Expander/Expander";
import { Pill } from "components/Pill/Pill";
import { colors } from "theme/colors";

export const Language = {
  licenseIssue: "License Issue",
  licenseIssues: (num: number): string => `${num} License Issues`,
  upgrade: "Contact sales@coder.com.",
  exceeded: "It looks like you've exceeded some limits of your license.",
  lessDetails: "Less",
  moreDetails: "More",
};

const styles = {
  leftContent: (theme) => ({
    marginRight: theme.spacing(1),
    marginLeft: theme.spacing(1),
  }),
} satisfies Record<string, Interpolation<Theme>>;

export interface LicenseBannerViewProps {
  errors: string[];
  warnings: string[];
}

export const LicenseBannerView: React.FC<LicenseBannerViewProps> = ({
  errors,
  warnings,
}) => {
  const theme = useTheme();
  const [showDetails, setShowDetails] = useState(false);
  const isError = errors.length > 0;
  const messages = [...errors, ...warnings];
  const type = isError ? "error" : "warning";

  const containerStyles = css`
    ${theme.typography.body2 as CSSObject}

    display: flex;
    align-items: center;
    padding: ${theme.spacing(1.5)};
    background-color: ${type === "error"
      ? colors.red[12]
      : theme.palette.warning.main};
  `;

  if (messages.length === 1) {
    return (
      <div css={containerStyles}>
        <Pill text={Language.licenseIssue} type={type} lightBorder />
        <div css={styles.leftContent}>
          <span>{messages[0]}</span>
          &nbsp;
          <Link color="white" fontWeight="medium" href="mailto:sales@coder.com">
            {Language.upgrade}
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div css={containerStyles}>
      <Pill
        text={Language.licenseIssues(messages.length)}
        type={type}
        lightBorder
      />
      <div css={styles.leftContent}>
        <div>
          {Language.exceeded}
          &nbsp;
          <Link color="white" fontWeight="medium" href="mailto:sales@coder.com">
            {Language.upgrade}
          </Link>
        </div>
        <Expander expanded={showDetails} setExpanded={setShowDetails}>
          <ul css={{ padding: theme.spacing(1), margin: 0 }}>
            {messages.map((message) => (
              <li css={{ margin: theme.spacing(0.5) }} key={message}>
                {message}
              </li>
            ))}
          </ul>
        </Expander>
      </div>
    </div>
  );
};
