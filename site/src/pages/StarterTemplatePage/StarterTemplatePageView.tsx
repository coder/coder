import Button from "@mui/material/Button";
import { makeStyles } from "@mui/styles";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { FC } from "react";
import { StarterTemplateContext } from "xServices/starterTemplates/starterTemplateXService";
import ViewCodeIcon from "@mui/icons-material/OpenInNewOutlined";
import PlusIcon from "@mui/icons-material/AddOutlined";
import { useTranslation } from "react-i18next";
import { Stack } from "components/Stack/Stack";
import { Link } from "react-router-dom";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export interface StarterTemplatePageViewProps {
  context: StarterTemplateContext;
}

export const StarterTemplatePageView: FC<StarterTemplatePageViewProps> = ({
  context,
}) => {
  const styles = useStyles();
  const { starterTemplate } = context;
  const { t } = useTranslation("starterTemplatePage");

  if (context.error) {
    return (
      <Margins>
        <ErrorAlert error={context.error} />
      </Margins>
    );
  }

  if (!starterTemplate) {
    return <Loader />;
  }

  return (
    <Margins>
      <PageHeader
        actions={
          <>
            <Button
              component="a"
              target="_blank"
              href={starterTemplate.url}
              rel="noreferrer"
              startIcon={<ViewCodeIcon />}
            >
              {t("actions.viewSourceCode")}
            </Button>
            <Button
              variant="contained"
              component={Link}
              to={`/templates/new?exampleId=${starterTemplate.id}`}
              startIcon={<PlusIcon />}
            >
              {t("actions.useTemplate")}
            </Button>
          </>
        }
      >
        <Stack direction="row" spacing={3} alignItems="center">
          <div className={styles.icon}>
            <img src={starterTemplate.icon} alt="" />
          </div>
          <div>
            <PageHeaderTitle>{starterTemplate.name}</PageHeaderTitle>
            <PageHeaderSubtitle condensed>
              {starterTemplate.description}
            </PageHeaderSubtitle>
          </div>
        </Stack>
      </PageHeader>

      <div className={styles.markdownSection} id="readme">
        <div className={styles.markdownWrapper}>
          <MemoizedMarkdown>{starterTemplate.markdown}</MemoizedMarkdown>
        </div>
      </div>
    </Margins>
  );
};

export const useStyles = makeStyles((theme) => {
  return {
    icon: {
      height: theme.spacing(6),
      width: theme.spacing(6),
      display: "flex",
      alignItems: "center",
      justifyContent: "center",

      "& img": {
        width: "100%",
      },
    },

    markdownSection: {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      borderRadius: theme.shape.borderRadius,
    },

    markdownWrapper: {
      padding: theme.spacing(5, 5, 8),
      maxWidth: 800,
      margin: "auto",
    },
  };
});
