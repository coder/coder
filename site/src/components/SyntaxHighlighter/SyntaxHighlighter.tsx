import { type ComponentProps, type FC } from "react";
import Editor, { DiffEditor, loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import { useCoderTheme } from "./coderTheme";

loader.config({ monaco });

interface SyntaxHighlighterProps {
  value: string;
  language: string;
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
