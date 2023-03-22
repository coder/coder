import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import EditIcon from "@material-ui/icons/Edit"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Loader } from "components/Loader/Loader"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderCaption,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { Stats, StatsItem } from "components/Stats/Stats"
import { TemplateFiles } from "components/TemplateFiles/TemplateFiles"
import { UseTabResult } from "hooks/useTab"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { Link as RouterLink } from "react-router-dom"
import { createDayString } from "util/createDayString"
import { TemplateVersionMachineContext } from "xServices/templateVersion/templateVersionXService"

export interface TemplateVersionPageViewProps {
  /**
   * Used to display the version name before loading the version in the API
   */
  versionName: string
  templateName: string
  canEdit: boolean
  tab: UseTabResult
  context: TemplateVersionMachineContext
}

export const TemplateVersionPageView: FC<TemplateVersionPageViewProps> = ({
  context,
  tab,
  versionName,
  templateName,
  canEdit,
}) => {
  const { currentFiles, error, currentVersion, previousFiles } = context
  const { t } = useTranslation("templateVersionPage")

  return (
    <Margins>
      <PageHeader
        actions={
          canEdit ? (
            <Link
              underline="none"
              component={RouterLink}
              to={`/templates/${templateName}/versions/${versionName}/edit`}
            >
              <Button variant="outlined" startIcon={<EditIcon />}>
                Edit
              </Button>
            </Link>
          ) : undefined
        }
      >
        <PageHeaderCaption>{t("header.caption")}</PageHeaderCaption>
        <PageHeaderTitle>{versionName}</PageHeaderTitle>
      </PageHeader>

      {!currentFiles && !error && <Loader />}

      <Stack spacing={4}>
        {Boolean(error) && <AlertBanner severity="error" error={error} />}
        {currentVersion && currentFiles && (
          <>
            <Stats>
              <StatsItem
                label={t("stats.template")}
                value={
                  <RouterLink to={`/templates/${templateName}`}>
                    {templateName}
                  </RouterLink>
                }
              />
              <StatsItem
                label={t("stats.createdBy")}
                value={currentVersion.created_by.username}
              />
              <StatsItem
                label={t("stats.created")}
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
  )
}

export default TemplateVersionPageView
