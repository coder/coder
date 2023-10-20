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
import ViewCodeIcon from "@mui/icons-material/OpenInNewOutlined";
import PlusIcon from "@mui/icons-material/AddOutlined";
import { Stack } from "components/Stack/Stack";
import { Link } from "react-router-dom";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { TemplateExample } from "api/typesGenerated";

export interface StarterTemplatePageViewProps {
  starterTemplate?: TemplateExample;
  error?: unknown;
}

export const StarterTemplatePageView: FC<StarterTemplatePageViewProps> = ({
  starterTemplate,
  error,
}) => {
  const styles = useStyles();

  if (error) {
    return (
      <Margins>
        <ErrorAlert error={error} />
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
              View source code
            </Button>
            <Button
              variant="contained"
              component={Link}
              to={`/templates/new?exampleId=${starterTemplate.id}`}
              startIcon={<PlusIcon />}
            >
              Use template
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
