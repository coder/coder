import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
import { MarkdownIcon } from "components/Icons/MarkdownIcon"
import { TerraformIcon } from "components/Icons/TerraformIcon"
import { Loader } from "components/Loader/Loader"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter"
import { useOrganizationId } from "hooks/useOrganizationId"
import { useParams, useSearchParams } from "react-router-dom"
import { combineClasses } from "util/combineClasses"
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
  const { version } = useParams() as Params
  const orgId = useOrganizationId()
  const [state] = useMachine(templateVersionMachine, {
    context: { versionName: version, orgId },
  })
  const { files } = state.context
  const fileTab = useFileTab()
  const styles = useStyles()

  return (
    <>
      <Margins>
        <PageHeader>
          <PageHeaderTitle>Version 18928cf</PageHeaderTitle>
          <PageHeaderSubtitle>coder-ts</PageHeaderSubtitle>
        </PageHeader>
      </Margins>

      {!files && !state.matches("done.error") && <Loader />}

      {state.matches("done.ok") && files && (
        <>
          <div className={styles.tabsWrapper}>
            <Margins>
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
            </Margins>
          </div>

          <div className={styles.codeWrapper}>
            <Margins>
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
            </Margins>
          </div>
        </>
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
  },

  tab: {
    background: "transparent",
    border: 0,
    padding: theme.spacing(0, 2),
    display: "flex",
    alignItems: "center",
    height: theme.spacing(5),
    opacity: 0.75,
    borderBottom: `1px solid ${theme.palette.divider}`,
    position: "relative",
    top: 1,
    cursor: "pointer",
    gap: theme.spacing(0.5),

    "& svg": {
      width: 22,
      maxHeight: 16,
    },
  },

  tabActive: {
    borderColor: theme.palette.secondary.dark,
    opacity: 1,
    fontWeight: 600,
  },

  codeWrapper: {
    background: theme.palette.background.paperLight,
  },

  prism: {
    paddingLeft: "0 !important",
    paddingRight: "0 !important",
  },
}))

export default TemplateVersionPage
