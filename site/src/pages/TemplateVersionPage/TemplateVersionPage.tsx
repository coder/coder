import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
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
import { useOrganizationId } from "hooks/useOrganizationId"
import { Link, useParams, useSearchParams } from "react-router-dom"
import { combineClasses } from "util/combineClasses"
import { createDayString } from "util/createDayString"
import { templateVersionMachine } from "xServices/templateVersion/templateVersionXService"

type Params = {
  version: string
  template: string
}

/**
 * Param used in the query string to locate the open file
 */
const FileTabParam = "file"

const useFileTab = () => {
  const [searchParams, setSearchParams] = useSearchParams()
  const value = searchParams.get(FileTabParam)

  return {
    value: value ? Number(value) : 0,
    set: (value: number) => {
      searchParams.set(FileTabParam, value.toString())
      setSearchParams(searchParams)
    },
  }
}

export const TemplateVersionPage: React.FC = () => {
  const { version: versionName, template } = useParams() as Params
  const orgId = useOrganizationId()
  const [state] = useMachine(templateVersionMachine, {
    context: { versionName, orgId },
  })
  const { files, version } = state.context
  const fileTab = useFileTab()
  const styles = useStyles()

  return (
    <>
      <Margins>
        <PageHeader>
          <PageHeaderCaption>Versions</PageHeaderCaption>
          <PageHeaderTitle>{versionName}</PageHeaderTitle>
        </PageHeader>
      </Margins>

      {!files && !state.matches("done.error") && <Loader />}

      {state.matches("done.ok") && version && files && (
        <Stack spacing={4}>
          <Margins>
            <Stats>
              <StatsItem
                label="Template"
                value={<Link to={`/templates/${template}`}>{template}</Link>}
              />
              <StatsItem
                label="Created by"
                value={version.created_by.username}
              />
              <StatsItem
                label="Created at"
                value={createDayString(version.created_at)}
              />
            </Stats>
          </Margins>

          <Margins>
            <div className={styles.files}>
              <div className={styles.tabs}>
                {Object.keys(files).map((filename, index) => {
                  return (
                    <button
                      className={combineClasses({
                        [styles.tab]: true,
                        [styles.tabActive]: index === fileTab.value,
                      })}
                      onClick={() => {
                        fileTab.set(index)
                      }}
                      key={filename}
                    >
                      {filename.endsWith("tf") ? (
                        <TerraformIcon />
                      ) : (
                        <MarkdownIcon />
                      )}
                      {filename}
                    </button>
                  )
                })}
              </div>

              <SyntaxHighlighter
                showLineNumbers
                className={styles.prism}
                language={
                  Object.keys(files)[fileTab.value].endsWith("tf")
                    ? "hcl"
                    : "markdown"
                }
              >
                {Object.values(files)[fileTab.value]}
              </SyntaxHighlighter>
            </div>
          </Margins>
        </Stack>
      )}
    </>
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
  },

  tab: {
    background: "transparent",
    border: 0,
    padding: theme.spacing(0, 3),
    display: "flex",
    alignItems: "center",
    height: theme.spacing(6),
    opacity: 0.75,
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
    fontWeight: 600,

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

export default TemplateVersionPage
