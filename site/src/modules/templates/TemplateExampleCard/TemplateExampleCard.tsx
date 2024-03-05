import type { Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import type { FC, HTMLAttributes } from "react";
import { Link as RouterLink } from "react-router-dom";
import type { TemplateExample } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Pill } from "components/Pill/Pill";

type TemplateExampleCardProps = HTMLAttributes<HTMLDivElement> & {
  example: TemplateExample;
  activeTag?: string;
};

export const TemplateExampleCard: FC<TemplateExampleCardProps> = ({
  example,
  activeTag,
  ...divProps
}) => {
  return (
    <div css={styles.card} {...divProps}>
      <div css={styles.header}>
        <div css={styles.icon}>
          <ExternalImage
            src={example.icon}
            css={{ width: "100%", height: "100%", objectFit: "contain" }}
          />
        </div>

        <div css={styles.tags}>
          {example.tags.map((tag) => (
            <RouterLink key={tag} to={`/starter-templates?tag=${tag}`}>
              <Pill css={[styles.tag, activeTag === tag && styles.activeTag]}>
                {tag}
              </Pill>
            </RouterLink>
          ))}
        </div>
      </div>

      <div>
        <h4 css={{ fontSize: 14, fontWeight: 600, margin: 0, marginBottom: 4 }}>
          {example.name}
        </h4>
        <span css={styles.description}>
          {example.description}{" "}
          <Link
            component={RouterLink}
            to={`/starter-templates/${example.id}`}
            css={{ display: "inline-block", fontSize: 13, marginTop: 4 }}
          >
            Read more
          </Link>
        </span>
      </div>

      <div css={styles.useButtonContainer}>
        <Button
          component={RouterLink}
          fullWidth
          to={`/templates/new?exampleId=${example.id}`}
        >
          Use template
        </Button>
      </div>
    </div>
  );
};

const styles = {
  card: (theme) => ({
    width: "320px",
    padding: 24,
    borderRadius: 6,
    border: `1px solid ${theme.palette.divider}`,
    textAlign: "left",
    color: "inherit",
    display: "flex",
    flexDirection: "column",
  }),

  header: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    marginBottom: 24,
  },

  icon: {
    flexShrink: 0,
    paddingTop: 4,
    width: 32,
    height: 32,
  },

  tags: {
    display: "flex",
    flexWrap: "wrap",
    gap: 8,
    justifyContent: "end",
  },

  tag: (theme) => ({
    borderColor: theme.palette.divider,
    textDecoration: "none",
    cursor: "pointer",
    "&: hover": {
      borderColor: theme.palette.primary.main,
    },
  }),

  activeTag: (theme) => ({
    borderColor: theme.roles.active.outline,
    backgroundColor: theme.roles.active.background,
  }),

  description: (theme) => ({
    fontSize: 13,
    color: theme.palette.text.secondary,
    lineHeight: "1.6",
    display: "block",
  }),

  useButtonContainer: {
    display: "flex",
    gap: 12,
    flexDirection: "column",
    paddingTop: 24,
    marginTop: "auto",
    alignItems: "center",
  },
} satisfies Record<string, Interpolation<Theme>>;
