import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import type { TemplateExample } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import { HTMLProps } from "react";
import { Link as RouterLink } from "react-router-dom";

type TemplateExampleCardProps = {
  example: TemplateExample;
  activeTag?: string;
} & HTMLProps<HTMLDivElement>;

export const TemplateExampleCard = (props: TemplateExampleCardProps) => {
  const { example, activeTag, ...divProps } = props;
  return (
    <div
      css={(theme) => ({
        width: "320px",
        padding: 24,
        borderRadius: 6,
        border: `1px solid ${theme.palette.divider}`,
        textAlign: "left",
        textDecoration: "none",
        color: "inherit",
        display: "flex",
        flexDirection: "column",
      })}
      {...divProps}
    >
      <div
        css={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 24,
        }}
      >
        <div
          css={{
            flexShrink: 0,
            paddingTop: 4,
            width: 32,
            height: 32,
          }}
        >
          <img
            src={example.icon}
            alt=""
            css={{ width: "100%", height: "100%", objectFit: "contain" }}
          />
        </div>

        <div css={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          {example.tags.map((tag) => {
            const isActive = activeTag === tag;

            return (
              <RouterLink key={tag} to={`/starter-templates?tag=${tag}`}>
                <Pill
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
                >
                  {tag}
                </Pill>
              </RouterLink>
            );
          })}
        </div>
      </div>

      <div>
        <h4 css={{ fontSize: 14, fontWeight: 600, margin: 0, marginBottom: 4 }}>
          {example.name}
        </h4>
        <span
          css={(theme) => ({
            fontSize: 13,
            color: theme.palette.text.secondary,
            lineHeight: "1.6",
            display: "block",
          })}
        >
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

      <div
        css={{
          display: "flex",
          gap: 12,
          flexDirection: "column",
          paddingTop: 24,
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
      </div>
    </div>
  );
};
