import { type Interpolation, type Theme } from "@emotion/react";
import type { TemplateExample } from "api/typesGenerated";
import { type FC } from "react";
import { Link } from "react-router-dom";

export interface TemplateExampleCardProps {
  example: TemplateExample;
  className?: string;
}

export const TemplateExampleCard: FC<TemplateExampleCardProps> = ({
  example,
  className,
}) => {
  return (
    <Link
      to={`/starter-templates/${example.id}`}
      css={styles.template}
      className={className}
      key={example.id}
    >
      <div css={styles.templateIcon}>
        <img src={example.icon} alt="" />
      </div>
      <div css={styles.templateInfo}>
        <span css={styles.templateName}>{example.name}</span>
        <span css={styles.templateDescription}>{example.description}</span>
      </div>
    </Link>
  );
};

const styles = {
  template: (theme) => ({
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
    background: theme.palette.background.paper,
    textDecoration: "none",
    textAlign: "left",
    color: "inherit",
    display: "flex",
    alignItems: "center",
    height: "fit-content",

    "&:hover": {
      backgroundColor: theme.palette.background.paperLight,
    },
  }),

  templateIcon: {
    width: 96,
    height: 96,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    flexShrink: 0,

    "& img": {
      height: 32,
    },
  },

  templateInfo: {
    padding: "16px 16px 16px 0",
    display: "flex",
    flexDirection: "column",
    overflow: "hidden",
  },

  templateName: {
    fontSize: 16,
    textOverflow: "ellipsis",
    width: "100%",
    overflow: "hidden",
    whiteSpace: "nowrap",
  },

  templateDescription: (theme) => ({
    fontSize: 14,
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    width: "100%",
    overflow: "hidden",
    whiteSpace: "nowrap",
  }),
} satisfies Record<string, Interpolation<Theme>>;
