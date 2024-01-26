import Button from "@mui/material/Button";
import { useTheme } from "@emotion/react";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { type FC } from "react";
import ViewCodeIcon from "@mui/icons-material/OpenInNewOutlined";
import PlusIcon from "@mui/icons-material/AddOutlined";
import { Stack } from "components/Stack/Stack";
import { Link } from "react-router-dom";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import type { TemplateExample } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";

export interface StarterTemplatePageViewProps {
  starterTemplate?: TemplateExample;
  error?: unknown;
}

export const StarterTemplatePageView: FC<StarterTemplatePageViewProps> = ({
  starterTemplate,
  error,
}) => {
  const theme = useTheme();

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
          <div
            css={{
              height: 48,
              width: 48,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",

              "& img": {
                width: "100%",
              },
            }}
          >
            <ExternalImage src={starterTemplate.icon} />
          </div>
          <div>
            <PageHeaderTitle>{starterTemplate.name}</PageHeaderTitle>
            <PageHeaderSubtitle condensed>
              {starterTemplate.description}
            </PageHeaderSubtitle>
          </div>
        </Stack>
      </PageHeader>

      <div
        css={{
          background: theme.palette.background.paper,
          border: `1px solid ${theme.palette.divider}`,
          borderRadius: 8,
        }}
        id="readme"
      >
        <div
          css={{
            padding: "40px 40px 64px",
            maxWidth: 800,
            margin: "auto",
          }}
        >
          <MemoizedMarkdown>{starterTemplate.markdown}</MemoizedMarkdown>
        </div>
      </div>
    </Margins>
  );
};
