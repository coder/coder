import { useTheme } from "@emotion/react";
import Editor, { DiffEditor, loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import type { ComponentProps, FC } from "react";
import { useCoderTheme } from "./coderTheme";

loader.config({ monaco });

interface SyntaxHighlighterProps {
  value: string;
  language?: string;
  editorProps?: ComponentProps<typeof Editor> &
    ComponentProps<typeof DiffEditor>;
  compareWith?: string;
}

export const SyntaxHighlighter: FC<SyntaxHighlighterProps> = ({
  value,
  compareWith,
  language,
  editorProps,
}) => {
  const hasDiff = compareWith && value !== compareWith;
  const theme = useTheme();
  const coderTheme = useCoderTheme();
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
    ...editorProps,
  };

  if (coderTheme.isLoading) {
    return null;
  }

  return (
    <div
      data-chromatic="ignore"
      css={{
        padding: "8px 0",
        height: "100%",
        backgroundColor: theme.monaco.colors["editor.background"],
      }}
    >
      {hasDiff ? (
        <DiffEditor original={compareWith} modified={value} {...commonProps} />
      ) : (
        <Editor value={value} {...commonProps} />
      )}
    </div>
  );
};
