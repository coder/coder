import Button from "@mui/material/Button";
import AddIcon from "@mui/icons-material/Add";
import EditIcon from "@mui/icons-material/Edit";
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
import { TemplateFiles } from "components/TemplateFiles/TemplateFiles";
import { UseTabResult } from "hooks/useTab";
import { type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { createDayString } from "utils/createDayString";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { TemplateVersion } from "api/typesGenerated";
import { TemplateVersionFiles } from "utils/templateVersion";

export interface TemplateVersionPageViewProps {
  versionName: string;
  templateName: string;
  tab: UseTabResult;
  createWorkspaceUrl?: string;
  error: unknown;
  currentVersion: TemplateVersion | undefined;
  currentFiles: TemplateVersionFiles | undefined;
  baseFiles: TemplateVersionFiles | undefined;
}

export const TemplateVersionPageView: FC<TemplateVersionPageViewProps> = ({
  tab,
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
        {currentVersion &&
          currentVersion.message &&
          currentVersion.message !== "" && (
            <PageHeaderSubtitle>{currentVersion.message}</PageHeaderSubtitle>
          )}
      </PageHeader>

      {!currentFiles && !error && <Loader />}

      <Stack spacing={4}>
        {Boolean(error) && <ErrorAlert error={error} />}
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
              tab={tab}
              currentFiles={currentFiles}
              baseFiles={baseFiles}
            />
          </>
        )}
      </Stack>
    </Margins>
  );
};

export default TemplateVersionPageView;
