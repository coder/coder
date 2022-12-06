import { FC } from "react"
import Editor, { DiffEditor } from "@monaco-editor/react"
import { useCoderTheme } from "./coderTheme"

export const SyntaxHighlighter: FC<{
  value: string
  language: string
  compareWith?: string
}> = ({ value, compareWith, language }) => {
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
      renderSideBySide: false,
      readOnly: true,
    },
  }

  if (coderTheme.isLoading) {
    return null
  }

  if (hasDiff) {
    return (
      <DiffEditor original={value} modified={compareWith} {...commonProps} />
    )
  }

  return <Editor value={value} {...commonProps} />
}
