import { useTheme } from "@material-ui/core/styles"
import Editor from "@monaco-editor/react"
import { FC, useEffect, useState } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import { hslToHex } from "util/colors"

export const MonacoEditor: FC<{
  value?: string
  language?: string
  onChange?: (value: string) => void
}> = ({ onChange, value, language }) => {
  const theme = useTheme()
  const [editor, setEditor] = useState<any>()
  useEffect(() => {
    if (!editor) {
      return
    }
    const resizeListener = () => {
      editor.layout({})
    }
    window.addEventListener("resize", resizeListener)
    return () => {
      window.removeEventListener("resize", resizeListener)
    }
  }, [editor])

  return (
    <Editor
      value={value || ""}
      language={language || "hcl"}
      theme="vs-dark"
      options={{
        automaticLayout: true,
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 16,
        wordWrap: "on",
      }}
      onChange={(newValue) => {
        if (onChange && newValue) {
          onChange(newValue)
        }
      }}
      onMount={(editor, monaco) => {
        setEditor(editor)

        document.fonts.ready
          .then(() => {
            // Ensures that all text is measured properly.
            // If this isn't done, there can be weird selection issues.
            monaco.editor.remeasureFonts()
          })
          .catch(() => {
            // Not a biggie!
          })

        monaco.editor.defineTheme("min", {
          base: "vs-dark",
          inherit: true,
          rules: [
            {
              token: "comment",
              foreground: "6B737C",
            },
            {
              token: "type",
              foreground: "B392F0",
            },
            {
              token: "string",
              foreground: "9DB1C5",
            },
            {
              token: "variable",
              foreground: "BBBBBB",
            },
            {
              token: "identifier",
              foreground: "B392F0",
            },
            {
              token: "delimiter.curly",
              foreground: "EBB325",
            },
          ],
          colors: {
            "editor.foreground": hslToHex(theme.palette.text.primary),
            "editor.background": hslToHex(theme.palette.background.paper),
          },
        })
        editor.updateOptions({
          theme: "min",
        })
      }}
    />
  )
}
