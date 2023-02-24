import { useTheme } from "@material-ui/core/styles"
import Editor from "@monaco-editor/react"
import { FC, useLayoutEffect, useMemo, useState } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import { hslToHex } from "util/colors"
import type { editor } from "monaco-editor"

export const MonacoEditor: FC<{
  value?: string
  path?: string
  onChange?: (value: string) => void
}> = ({ onChange, value, path }) => {
  const theme = useTheme()
  const [editor, setEditor] = useState<editor.IStandaloneCodeEditor>()
  useLayoutEffect(() => {
    if (!editor) {
      return
    }
    const resizeListener = () => {
      editor.layout({
        height: 0,
        width: 0,
      })
    }
    window.addEventListener("resize", resizeListener)
    return () => {
      window.removeEventListener("resize", resizeListener)
    }
  }, [editor])

  const language = useMemo(() => {
    if (path?.endsWith(".tf")) {
      return "hcl"
    }
    if (path?.endsWith(".md")) {
      return "markdown"
    }
    if (path?.endsWith(".json")) {
      return "json"
    }
    if (path?.endsWith(".yaml")) {
      return "yaml"
    }
    if (path?.endsWith("Dockerfile")) {
      return "dockerfile"
    }
  }, [path])

  return (
    <Editor
      value={value}
      language={language}
      theme="vs-dark"
      options={{
        automaticLayout: true,
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 16,
        wordWrap: "on",
        padding: {
          top: 16,
          bottom: 16,
        },
      }}
      path={path}
      onChange={(newValue) => {
        if (onChange && newValue) {
          onChange(newValue)
        }
      }}
      onMount={(editor, monaco) => {
        // This jank allows for Ctrl + Enter to work outside the editor.
        // We use this keybind to trigger a build.
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Private type in Monaco!
        ;(editor as any)._standaloneKeybindingService.addDynamicKeybinding(
          `-editor.action.insertLineAfter`,
          undefined,
          () => {
            //
          },
        )

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
            "editor.background": hslToHex(theme.palette.background.default),
          },
        })
        editor.updateOptions({
          theme: "min",
        })
      }}
    />
  )
}
