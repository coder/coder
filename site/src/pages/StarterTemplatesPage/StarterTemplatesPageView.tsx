import { makeStyles } from "@mui/styles";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Maybe } from "components/Conditionals/Maybe";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { TemplateExampleCard } from "components/TemplateExampleCard/TemplateExampleCard";
import { FC } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { combineClasses } from "utils/combineClasses";
import { StarterTemplatesContext } from "xServices/starterTemplates/starterTemplatesXService";

const getTagLabel = (tag: string, t: (key: string) => string) => {
  const labelByTag: Record<string, string> = {
    all: t("tags.all"),
    digitalocean: t("tags.digitalocean"),
    aws: t("tags.aws"),
    google: t("tags.google"),
  };
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- this can be undefined
  return labelByTag[tag] ?? tag;
};

const selectTags = ({ starterTemplatesByTag }: StarterTemplatesContext) => {
  return starterTemplatesByTag
    ? Object.keys(starterTemplatesByTag).sort((a, b) => a.localeCompare(b))
    : undefined;
};
export interface StarterTemplatesPageViewProps {
  context: StarterTemplatesContext;
}

export const StarterTemplatesPageView: FC<StarterTemplatesPageViewProps> = ({
  context,
}) => {
  const { t } = useTranslation("starterTemplatesPage");
  const [urlParams] = useSearchParams();
  const styles = useStyles();
  const { starterTemplatesByTag } = context;
  const tags = selectTags(context);
  const activeTag = urlParams.get("tag") ?? "all";
  const visibleTemplates = starterTemplatesByTag
    ? starterTemplatesByTag[activeTag]
    : undefined;

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>{t("title")}</PageHeaderTitle>
        <PageHeaderSubtitle>{t("subtitle")}</PageHeaderSubtitle>
      </PageHeader>

      <Maybe condition={Boolean(context.error)}>
        <ErrorAlert error={context.error} />
      </Maybe>

      <Maybe condition={Boolean(!starterTemplatesByTag)}>
        <Loader />
      </Maybe>

      <Stack direction="row" spacing={4}>
        {starterTemplatesByTag && tags && (
          <Stack className={styles.filter}>
            <span className={styles.filterCaption}>{t("filterCaption")}</span>
            {tags.map((tag) => (
              <Link
                key={tag}
                to={`?tag=${tag}`}
                className={combineClasses({
                  [styles.tagLink]: true,
                  [styles.tagLinkActive]: tag === activeTag,
                })}
              >
                {getTagLabel(tag, t)} ({starterTemplatesByTag[tag].length})
              </Link>
            ))}
          </Stack>
        )}

        <div className={styles.templates}>
          {visibleTemplates &&
            visibleTemplates.map((example) => (
              <TemplateExampleCard example={example} key={example.id} />
            ))}
        </div>
      </Stack>
    </Margins>
  );
};

const useStyles = makeStyles((theme) => ({
  filter: {
    width: theme.spacing(26),
    flexShrink: 0,
  },

  filterCaption: {
    textTransform: "uppercase",
    fontWeight: 600,
    fontSize: 12,
    color: theme.palette.text.secondary,
    letterSpacing: "0.1em",
  },

  tagLink: {
    color: theme.palette.text.secondary,
    textDecoration: "none",
    fontSize: 14,
    textTransform: "capitalize",

    "&:hover": {
      color: theme.palette.text.primary,
    },
  },

  tagLinkActive: {
    color: theme.palette.text.primary,
    fontWeight: 600,
  },

  templates: {
    flex: "1",
    display: "grid",
    gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
    gap: theme.spacing(2),
    gridAutoRows: "min-content",
  },
}));
