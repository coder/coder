import AddIcon from "@mui/icons-material/Add";
import EditIcon from "@mui/icons-material/Edit";
import Button from "@mui/material/Button";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import type { TemplateVersion } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderCaption,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { Stats, StatsItem } from "components/Stats/Stats";
import { TemplateFiles } from "modules/templates/TemplateFiles/TemplateFiles";
import { TemplateUpdateMessage } from "modules/templates/TemplateUpdateMessage";
import { createDayString } from "utils/createDayString";
import type { TemplateVersionFiles } from "utils/templateVersion";

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
          <TemplateUpdateMessage>
            {currentVersion.message}
          </TemplateUpdateMessage>
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

export default TemplateVersionPageView;
