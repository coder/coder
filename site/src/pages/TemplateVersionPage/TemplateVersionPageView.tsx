import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import EditIcon from "@material-ui/icons/Edit"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { DockerIcon } from "components/Icons/DockerIcon"
import { MarkdownIcon } from "components/Icons/MarkdownIcon"
import { TerraformIcon } from "components/Icons/TerraformIcon"
import { Loader } from "components/Loader/Loader"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderCaption,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { Stats, StatsItem } from "components/Stats/Stats"
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter"
import { UseTabResult } from "hooks/useTab"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { Link as RouterLink } from "react-router-dom"
import { combineClasses } from "util/combineClasses"
import { createDayString } from "util/createDayString"
import { TemplateVersionFiles } from "util/templateVersion"
import { TemplateVersionMachineContext } from "xServices/templateVersion/templateVersionXService"

const iconByExtension: Record<string, JSX.Element> = {
  tf: <TerraformIcon />,
  md: <MarkdownIcon />,
  mkd: <MarkdownIcon />,
  Dockerfile: <DockerIcon />,
}

const getExtension = (filename: string) => {
  if (filename.includes(".")) {
    const [_, extension] = filename.split(".")
    return extension
  }

  return filename
}

const languageByExtension: Record<string, string> = {
  tf: "hcl",
  md: "markdown",
  mkd: "markdown",
  Dockerfile: "dockerfile",
}

const Files: FC<{
  currentFiles: TemplateVersionFiles
  previousFiles?: TemplateVersionFiles
  tab: UseTabResult
}> = ({ currentFiles, previousFiles, tab }) => {
  const styles = useStyles()
  const filenames = Object.keys(currentFiles)
  const selectedFilename = filenames[Number(tab.value)]
  const currentFile = currentFiles[selectedFilename]
  const previousFile = previousFiles && previousFiles[selectedFilename]

  return (
    <div className={styles.files}>
      <div className={styles.tabs}>
        {filenames.map((filename, index) => {
          const tabValue = index.toString()
          const extension = getExtension(filename)
          const icon = iconByExtension[extension]
          const hasDiff =
            previousFiles &&
            previousFiles[filename] &&
            currentFiles[filename] !== previousFiles[filename]

          return (
            <button
              className={combineClasses({
                [styles.tab]: true,
                [styles.tabActive]: tabValue === tab.value,
              })}
              onClick={() => {
                tab.set(tabValue)
              }}
              key={filename}
            >
              {icon}
              {filename}
              {hasDiff && <div className={styles.tabDiff} />}
            </button>
          )
        })}
      </div>

      <SyntaxHighlighter
        value={currentFile}
        compareWith={previousFile}
        language={languageByExtension[getExtension(selectedFilename)]}
      />
    </div>
  )
}
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

            <Files
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

const useStyles = makeStyles((theme) => ({
  tabsWrapper: {
    borderBottom: `1px solid ${theme.palette.divider}`,
  },

  tabs: {
    display: "flex",
    alignItems: "baseline",
    borderBottom: `1px solid ${theme.palette.divider}`,
    gap: 1,
  },

  tab: {
    background: "transparent",
    border: 0,
    padding: theme.spacing(0, 3),
    display: "flex",
    alignItems: "center",
    height: theme.spacing(6),
    opacity: 0.85,
    cursor: "pointer",
    gap: theme.spacing(0.5),
    position: "relative",

    "& svg": {
      width: 22,
      maxHeight: 16,
    },

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  },

  tabActive: {
    opacity: 1,
    background: theme.palette.action.hover,

    "&:after": {
      content: '""',
      display: "block",
      height: 1,
      width: "100%",
      bottom: 0,
      left: 0,
      backgroundColor: theme.palette.secondary.dark,
      position: "absolute",
    },
  },

  tabDiff: {
    height: 6,
    width: 6,
    backgroundColor: theme.palette.warning.light,
    borderRadius: "100%",
    marginLeft: theme.spacing(0.5),
  },

  codeWrapper: {
    background: theme.palette.background.paperLight,
  },

  files: {
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
  },

  prism: {
    borderRadius: 0,
  },
}))

export default TemplateVersionPageView
