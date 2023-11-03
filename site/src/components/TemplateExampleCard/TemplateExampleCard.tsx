import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import type { TemplateExample } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import { type FC } from "react";
import { Link as RouterLink } from "react-router-dom";

export interface TemplateExampleCardProps {
  example: TemplateExample;
  activeTag?: string;
}

export const TemplateExampleCard: FC<TemplateExampleCardProps> = ({
  example,
  activeTag,
}) => {
  return (
    <div
      css={(theme) => ({
        width: "320px",
        padding: theme.spacing(3),
        borderRadius: 6,
        border: `1px solid ${theme.palette.divider}`,
        textAlign: "left",
        textDecoration: "none",
        color: "inherit",
        display: "flex",
        flexDirection: "column",
      })}
    >
      <div
        css={{
          flexShrink: 0,
          paddingTop: 4,
          width: 32,
          height: 32,
          marginBottom: 16,
        }}
      >
        <img
          src={example.icon}
          alt=""
          css={{ width: "100%", height: "100%", objectFit: "contain" }}
        />
      </div>

      <div>
        <h4 css={{ fontSize: 14, fontWeight: 600, margin: 0 }}>
          {example.name}
        </h4>
        <span
          css={(theme) => ({
            fontSize: 13,
            color: theme.palette.text.secondary,
            lineHeight: "0.5",
          })}
        >
          {example.description}
        </span>
        <div css={{ marginTop: 16, display: "flex", flexWrap: "wrap", gap: 8 }}>
          {example.tags.map((tag) => {
            const isActive = activeTag === tag;

            return (
              <RouterLink key={tag} to={`/starter-templates?tag=${tag}`}>
                <Pill
                  text={tag}
                  css={(theme) => ({
                    borderColor: isActive
                      ? theme.palette.primary.main
                      : theme.palette.divider,
                    cursor: "pointer",
                    backgroundColor: isActive
                      ? theme.palette.primary.dark
                      : undefined,
                    "&: hover": {
                      borderColor: theme.palette.primary.main,
                    },
                  })}
                />
              </RouterLink>
            );
          })}
        </div>
      </div>

      <div
        css={{
          display: "flex",
          gap: 12,
          flexDirection: "column",
          paddingTop: 32,
          marginTop: "auto",
          alignItems: "center",
        }}
      >
        <Button
          component={RouterLink}
          fullWidth
          to={`/templates/new?exampleId=${example.id}`}
        >
          Use template
        </Button>
        <Link
          component={RouterLink}
          css={{ fontSize: 13 }}
          to={`/starter-templates/${example.id}`}
        >
          Read more
        </Link>
      </div>
    </div>
  );
};
