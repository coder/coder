import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
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
import { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { createDayString } from "utils/createDayString";
import { TemplateVersionMachineContext } from "xServices/templateVersion/templateVersionXService";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export interface TemplateVersionPageViewProps {
  /**
   * Used to display the version name before loading the version in the API
   */
  versionName: string;
  templateName: string;
  tab: UseTabResult;
  context: TemplateVersionMachineContext;
}

export const TemplateVersionPageView: FC<TemplateVersionPageViewProps> = ({
  context,
  tab,
  versionName,
  templateName,
}) => {
  const { currentFiles, error, currentVersion, previousFiles } = context;

  return (
    <Margins>
      <PageHeader
        actions={
          <Link
            component={RouterLink}
            to={`/templates/${templateName}/versions/${versionName}/edit`}
          >
            <Button startIcon={<EditIcon />}>Edit</Button>
          </Link>
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
              previousFiles={previousFiles}
            />
          </>
        )}
      </Stack>
    </Margins>
  );
};

export default TemplateVersionPageView;
