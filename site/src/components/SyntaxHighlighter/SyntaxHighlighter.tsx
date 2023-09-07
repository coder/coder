import { ComponentProps, FC } from "react";
import Editor, { DiffEditor, loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import { useCoderTheme } from "./coderTheme";
import { makeStyles } from "@mui/styles";

loader.config({ monaco });

export const SyntaxHighlighter: FC<{
  value: string;
  language: string;
  editorProps?: ComponentProps<typeof Editor> &
    ComponentProps<typeof DiffEditor>;
  compareWith?: string;
}> = ({ value, compareWith, language, editorProps }) => {
  const styles = useStyles();
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
    <div className={styles.wrapper}>
      {hasDiff ? (
        <DiffEditor original={compareWith} modified={value} {...commonProps} />
      ) : (
        <Editor value={value} {...commonProps} />
      )}
    </div>
  );
};

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(1, 0),
    background: theme.palette.background.paper,
    height: "100%",
  },
}));
