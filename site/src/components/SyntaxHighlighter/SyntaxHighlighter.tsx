import { FC } from "react"
import Editor, { DiffEditor } from "@monaco-editor/react"
import { useCoderTheme } from "./coderTheme"
import { makeStyles } from "@material-ui/core/styles"

export const SyntaxHighlighter: FC<{
  value: string
  language: string
  compareWith?: string
}> = ({ value, compareWith, language }) => {
  const styles = useStyles()
  const hasDiff = compareWith && value !== compareWith
  const coderTheme = useCoderTheme()
  const commonProps = {
    language,
    theme: coderTheme.name,
    height: 560,
    options: {
      minimap: {
        enabled: false,
      },
      renderSideBySide: true,
      readOnly: true,
    },
  }

  if (coderTheme.isLoading) {
    return null
  }

  return (
    <div className={styles.wrapper}>
      {hasDiff ? (
        <DiffEditor original={compareWith} modified={value} {...commonProps} />
      ) : (
        <Editor value={value} {...commonProps} />
      )}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(1, 0),
    background: theme.palette.background.paper,
  },
}))
