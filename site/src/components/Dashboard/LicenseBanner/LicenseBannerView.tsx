import Link from "@mui/material/Link";
import {
  css,
  type CSSObject,
  type Interpolation,
  type Theme,
  useTheme,
} from "@emotion/react";
import { type FC, useState } from "react";
import { Expander } from "components/Expander/Expander";
import { Pill } from "components/Pill/Pill";

export const Language = {
  licenseIssue: "License Issue",
  licenseIssues: (num: number): string => `${num} License Issues`,
  upgrade: "Contact sales@coder.com.",
  exceeded: "It looks like you've exceeded some limits of your license.",
  lessDetails: "Less",
  moreDetails: "More",
};

const styles = {
  leftContent: {
    marginRight: 8,
    marginLeft: 8,
  },
} satisfies Record<string, Interpolation<Theme>>;

export interface LicenseBannerViewProps {
  errors: string[];
  warnings: string[];
}

export const LicenseBannerView: FC<LicenseBannerViewProps> = ({
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
    padding: 12px;
    background-color: ${theme.experimental.roles[type].background};
  `;

  const textColor = theme.experimental.roles[type].text;

  if (messages.length === 1) {
    return (
      <div css={containerStyles}>
        <Pill type={type}>{Language.licenseIssue}</Pill>
        <div css={styles.leftContent}>
          <span>{messages[0]}</span>
          &nbsp;
          <Link
            color={textColor}
            fontWeight="medium"
            href="mailto:sales@coder.com"
          >
            {Language.upgrade}
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div css={containerStyles}>
      <Pill type={type}>{Language.licenseIssues(messages.length)}</Pill>
      <div css={styles.leftContent}>
        <div>
          {Language.exceeded}
          &nbsp;
          <Link
            color={textColor}
            fontWeight="medium"
            href="mailto:sales@coder.com"
          >
            {Language.upgrade}
          </Link>
        </div>
        <Expander expanded={showDetails} setExpanded={setShowDetails}>
          <ul css={{ padding: 8, margin: 0 }}>
            {messages.map((message) => (
              <li css={{ margin: 4 }} key={message}>
                {message}
              </li>
            ))}
          </ul>
        </Expander>
      </div>
    </div>
  );
};
