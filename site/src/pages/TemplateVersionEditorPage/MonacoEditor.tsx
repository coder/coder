import { useTheme } from "@emotion/react";
import Editor, { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import { type FC, useEffect } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";

loader.config({ monaco });

export interface MonacoEditorProps {
  value?: string;
  path?: string;
  onChange?: (value: string) => void;
}

export const MonacoEditor: FC<MonacoEditorProps> = ({
  onChange,
  value,
  path,
}) => {
  const theme = useTheme();

  useEffect(() => {
    document.fonts.ready
      .then(() => {
        // Ensures that all text is measured properly.
        // If this isn't done, there can be weird selection issues.
        monaco.editor.remeasureFonts();
      })
      .catch(() => {
        // Not a biggie!
      });

    monaco.editor.defineTheme("min", theme.monaco);
  }, [theme]);

  return (
    <Editor
      value={value}
      theme="vs-dark"
      options={{
        automaticLayout: true,
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 14,
        wordWrap: "on",
        padding: {
          top: 16,
          bottom: 16,
        },
      }}
      path={path}
      onChange={(newValue) => {
        if (onChange && newValue) {
          onChange(newValue);
        }
      }}
      onMount={(editor) => {
        // This jank allows for Ctrl + Enter to work outside the editor.
        // We use this keybind to trigger a build.
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Private type in Monaco!
        (editor as any)._standaloneKeybindingService.addDynamicKeybinding(
          `-editor.action.insertLineAfter`,
          monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter,
          () => {},
        );

        editor.updateOptions({
          theme: "min",
        });
      }}
    />
  );
};
