import { useTheme } from "@emotion/react";
import frontMatter from "front-matter";
import { Helmet } from "react-helmet-async";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { pageTitle } from "utils/page";

export default function TemplateDocsPage() {
  const { template, activeVersion } = useTemplateLayoutContext();
  const theme = useTheme();

  const readme = frontMatter(activeVersion.readme);

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Documentation`)}</title>
      </Helmet>

      <div
        css={{
          background: theme.palette.background.paper,
          border: `1px solid ${theme.palette.divider}`,
          borderRadius: theme.shape.borderRadius,
        }}
        id="readme"
      >
        <div
          css={{
            color: theme.palette.text.secondary,
            fontWeight: 600,
            padding: theme.spacing(2, 3),
            borderBottom: `1px solid ${theme.palette.divider}`,
          }}
        >
          README.md
        </div>
        <div
          css={{
            padding: theme.spacing(0, 3, 5),
            maxWidth: 800,
            margin: "auto",
          }}
        >
          <MemoizedMarkdown>{readme.body}</MemoizedMarkdown>
        </div>
      </div>
    </>
  );
}
