import Button from "@mui/material/Button";
import AddIcon from "@mui/icons-material/Add";
import EditIcon from "@mui/icons-material/Edit";
import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderCaption,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { Stats, StatsItem } from "components/Stats/Stats";
import { TemplateFiles } from "modules/templates/TemplateFiles/TemplateFiles";
import type { TemplateVersion } from "api/typesGenerated";
import { createDayString } from "utils/createDayString";
import { TemplateVersionFiles } from "utils/templateVersion";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { MemoizedMarkdown } from "components/Markdown/Markdown";

export interface TemplateVersionPageViewProps {
  versionName: string;
  templateName: string;
  createWorkspaceUrl?: string;
  error: unknown;
  currentVersion: TemplateVersion | undefined;
  currentFiles: TemplateVersionFiles | undefined;
  baseFiles: TemplateVersionFiles | undefined;
}

export const TemplateVersionPageView: FC<TemplateVersionPageViewProps> = ({
  versionName,
  templateName,
  createWorkspaceUrl,
  currentVersion,
  currentFiles,
  baseFiles,
  error,
}) => {
  return (
    <Margins>
      <PageHeader
        actions={
          <>
            {createWorkspaceUrl && (
              <Button
                variant="contained"
                startIcon={<AddIcon />}
                component={RouterLink}
                to={createWorkspaceUrl}
              >
                Create workspace
              </Button>
            )}
            <Button
              startIcon={<EditIcon />}
              component={RouterLink}
              to={`/templates/${templateName}/versions/${versionName}/edit`}
            >
              Edit
            </Button>
          </>
        }
      >
        <PageHeaderCaption>Version</PageHeaderCaption>
        <PageHeaderTitle>{versionName}</PageHeaderTitle>
      </PageHeader>

      {!currentFiles && !error && <Loader />}

      <Stack spacing={4}>
        {Boolean(error) && <ErrorAlert error={error} />}
        {currentVersion?.message && (
          <MemoizedMarkdown css={styles.versionMessage}>
            {currentVersion.message}
          </MemoizedMarkdown>
        )}
        {currentVersion && currentFiles && (
          <>
            <Stats>
              <StatsItem
                label="Template"
                value={
                  <RouterLink to={`/templates/${templateName}`}>
                    {templateName}
                  </RouterLink>
                }
              />
              <StatsItem
                label="Created by"
                value={currentVersion.created_by.username}
              />
              <StatsItem
                label="Created"
                value={createDayString(currentVersion.created_at)}
              />
            </Stats>

            <TemplateFiles
              currentFiles={currentFiles}
              baseFiles={baseFiles}
              templateName={templateName}
              versionName={versionName}
            />
          </>
        )}
      </Stack>
    </Margins>
  );
};

const styles = {
  versionMessage: {
    fontSize: "0.9em",
    lineHeight: 1.2,

    "& h1, & h2, & h3, & h4, & h5, & h6": {
      margin: "0 0 0.75em",
    },
    "& h1": {
      fontSize: "1.2em",
    },
    "& h2": {
      fontSize: "1.15em",
    },
    "& h3": {
      fontSize: "1.1em",
    },
    "& h4": {
      fontSize: "1.05em",
    },
    "& h5": {
      fontSize: "1em",
    },
    "& h6": {
      fontSize: "0.95em",
    },
  },
} satisfies Record<string, Interpolation<Theme>>;

export default TemplateVersionPageView;
