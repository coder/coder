import { makeStyles } from "@material-ui/core/styles"
import { DockerIcon } from "components/Icons/DockerIcon"
import { MarkdownIcon } from "components/Icons/MarkdownIcon"
import { TerraformIcon } from "components/Icons/TerraformIcon"
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter"
import { UseTabResult } from "hooks/useTab"
import { FC } from "react"
import { combineClasses } from "util/combineClasses"
import { TemplateVersionFiles } from "util/templateVersion"

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

export const TemplateFiles: FC<{
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
const useStyles = makeStyles((theme) => ({
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
