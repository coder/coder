import type { Interpolation, Theme } from "@emotion/react";
import type { FC } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Stack } from "components/Stack/Stack";
import { TemplateExampleCard } from "modules/templates/TemplateExampleCard/TemplateExampleCard";
import type { StarterTemplatesByTag } from "utils/templateAggregators";

const getTagLabel = (tag: string) => {
  const labelByTag: Record<string, string> = {
    all: "All templates",
    digitalocean: "DigitalOcean",
    aws: "AWS",
    google: "Google Cloud",
  };
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- this can be undefined
  return labelByTag[tag] ?? tag;
};

const selectTags = (starterTemplatesByTag: StarterTemplatesByTag) => {
  return starterTemplatesByTag
    ? Object.keys(starterTemplatesByTag).sort((a, b) => a.localeCompare(b))
    : undefined;
};

export interface StarterTemplatesProps {
  starterTemplatesByTag?: StarterTemplatesByTag;
}

export const StarterTemplates: FC<StarterTemplatesProps> = ({
  starterTemplatesByTag,
}) => {
  const [urlParams] = useSearchParams();
  const tags = starterTemplatesByTag
    ? selectTags(starterTemplatesByTag)
    : undefined;
  const activeTag = urlParams.get("tag") ?? "all";
  const visibleTemplates = starterTemplatesByTag
    ? starterTemplatesByTag[activeTag]
    : undefined;

  return (
    <Stack direction="row" spacing={4} alignItems="flex-start">
      {starterTemplatesByTag && tags && (
        <Stack css={{ width: 202, flexShrink: 0, position: "sticky" }}>
          <h2 css={styles.sectionTitle}>Choose a starter template</h2>
          <span css={styles.filterCaption}>Filter</span>
          {tags.map((tag) => (
            <Link
              key={tag}
              to={`?tag=${tag}`}
              css={[styles.tagLink, tag === activeTag && styles.tagLinkActive]}
            >
              {getTagLabel(tag)} ({starterTemplatesByTag[tag].length})
            </Link>
          ))}
        </Stack>
      )}

      <div
        css={{
          display: "flex",
          flexWrap: "wrap",
          gap: 32,
          height: "max-content",
        }}
      >
        {visibleTemplates &&
          visibleTemplates.map((example) => (
            <TemplateExampleCard
              css={(theme) => ({
                backgroundColor: theme.palette.background.paper,
              })}
              example={example}
              key={example.id}
              activeTag={activeTag}
            />
          ))}
      </div>
    </Stack>
  );
};

const styles = {
  filterCaption: (theme) => ({
    textTransform: "uppercase",
    fontWeight: 600,
    fontSize: 12,
    color: theme.palette.text.secondary,
    letterSpacing: "0.1em",
  }),

  tagLink: (theme) => ({
    color: theme.palette.text.secondary,
    textDecoration: "none",
    fontSize: 14,
    textTransform: "capitalize",

    "&:hover": {
      color: theme.palette.text.primary,
    },
  }),

  tagLinkActive: (theme) => ({
    color: theme.palette.text.primary,
    fontWeight: 600,
  }),

  sectionTitle: (theme) => ({
    color: theme.palette.text.primary,
    fontSize: 16,
    fontWeight: 400,
    margin: 0,
  }),
} satisfies Record<string, Interpolation<Theme>>;
