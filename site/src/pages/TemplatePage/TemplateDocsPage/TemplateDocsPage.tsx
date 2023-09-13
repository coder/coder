import { makeStyles } from "@mui/styles";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout";
import frontMatter from "front-matter";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";

export default function TemplateDocsPage() {
  const { template, activeVersion } = useTemplateLayoutContext();
  const styles = useStyles();

  const readme = frontMatter(activeVersion.readme);

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Documentation`)}</title>
      </Helmet>

      <div className={styles.markdownSection} id="readme">
        <div className={styles.readmeLabel}>README.md</div>
        <div className={styles.markdownWrapper}>
          <MemoizedMarkdown>{readme.body}</MemoizedMarkdown>
        </div>
      </div>
    </>
  );
}

export const useStyles = makeStyles((theme) => {
  return {
    markdownSection: {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      borderRadius: theme.shape.borderRadius,
    },

    readmeLabel: {
      color: theme.palette.text.secondary,
      fontWeight: 600,
      padding: theme.spacing(2, 3),
      borderBottom: `1px solid ${theme.palette.divider}`,
    },

    markdownWrapper: {
      padding: theme.spacing(0, 3, 5),
      maxWidth: 800,
      margin: "auto",
    },
  };
});
