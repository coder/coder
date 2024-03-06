import type { Interpolation, Theme } from "@emotion/react";
import type { FC } from "react";
import { MemoizedMarkdown } from "components/Markdown/Markdown";

interface TemplateUpdateMessageProps {
  children: string;
}

export const TemplateUpdateMessage: FC<TemplateUpdateMessageProps> = ({
  children,
}) => {
  return (
    <MemoizedMarkdown css={styles.versionMessage}>{children}</MemoizedMarkdown>
  );
};

const styles = {
  versionMessage: {
    fontSize: 14,
    lineHeight: 1.2,

    "& h1, & h2, & h3, & h4, & h5, & h6": {
      margin: "0 0 0.75em",
    },
    "& h1": {
      fontSize: "1.2em",
    },
    "& h2": {
      fontSize: "1.15em",
    },
    "& h3": {
      fontSize: "1.1em",
    },
    "& h4": {
      fontSize: "1.05em",
    },
    "& h5": {
      fontSize: "1em",
    },
    "& h6": {
      fontSize: "0.95em",
    },
  },
} satisfies Record<string, Interpolation<Theme>>;
