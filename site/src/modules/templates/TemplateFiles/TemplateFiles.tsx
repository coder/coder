import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter";
import { TemplateVersionFiles } from "utils/templateVersion";
import RadioButtonCheckedOutlined from "@mui/icons-material/RadioButtonCheckedOutlined";
import { Pill } from "components/Pill/Pill";
import { Link } from "react-router-dom";

const languageByExtension: Record<string, string> = {
  tf: "hcl",
  hcl: "hcl",
  md: "markdown",
  mkd: "markdown",
  Dockerfile: "dockerfile",
  sh: "shell",
  tpl: "tpl",
  protobuf: "protobuf",
  nix: "dockerfile",
};
interface TemplateFilesProps {
  currentFiles: TemplateVersionFiles;
  /**
   * Files used to compare with current files
   */
  baseFiles?: TemplateVersionFiles;
}

export const TemplateFiles: FC<TemplateFilesProps> = ({
  currentFiles,
  baseFiles,
}) => {
  const filenames = Object.keys(currentFiles);
  const theme = useTheme();
  const filesWithDiff = filenames.filter(
    (filename) => fileInfo(filename).hasDiff,
  );

  function fileInfo(filename: string) {
    const value = currentFiles[filename].trim();
    const previousValue = baseFiles ? baseFiles[filename].trim() : undefined;
    const hasDiff = previousValue && value !== previousValue;

    return {
      value,
      previousValue,
      hasDiff,
    };
  }

  return (
    <div>
      {filesWithDiff.length > 0 && (
        <div
          css={{
            display: "flex",
            alignItems: "center",
            gap: 16,
            marginBottom: 24,
          }}
        >
          <span
            css={(theme) => ({
              fontSize: 13,
              fontWeight: 500,
              color: theme.roles.warning.fill.outline,
            })}
          >
            {filesWithDiff.length} files have changes
          </span>
          <ul
            css={{
              listStyle: "none",
              margin: 0,
              padding: 0,
              display: "flex",
              alignItems: "center",
              gap: 4,
            }}
          >
            {filesWithDiff.map((filename) => (
              <li key={filename}>
                <a
                  href={`#${encodeURIComponent(filename)}`}
                  css={{
                    textDecoration: "none",
                    color: theme.roles.warning.fill.text,
                    fontSize: 13,
                    fontWeight: 500,
                    backgroundColor: theme.roles.warning.background,
                    display: "inline-block",
                    padding: "0 8px",
                    borderRadius: 4,
                    border: `1px solid ${theme.roles.warning.fill.solid}`,
                    lineHeight: "1.6",
                  }}
                >
                  {filename}
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div css={styles.files}>
        {[...filenames]
          .sort((a, b) => a.localeCompare(b))
          .map((filename) => {
            const info = fileInfo(filename);

            return (
              <div key={filename} css={styles.filePanel} id={filename}>
                <header css={styles.fileHeader}>
                  {filename}
                  {info.hasDiff && (
                    <RadioButtonCheckedOutlined
                      css={{
                        width: 14,
                        height: 14,
                        color: theme.roles.warning.fill.outline,
                      }}
                    />
                  )}
                </header>
                <SyntaxHighlighter
                  language={
                    languageByExtension[filename.split(".").pop() ?? ""]
                  }
                  value={info.value}
                  compareWith={info.previousValue}
                  editorProps={{
                    // 18 is the editor line height
                    height: Math.min(numberOfLines(info.value) * 18, 560),
                    onMount: (editor) => {
                      editor.updateOptions({
                        scrollBeyondLastLine: false,
                      });
                    },
                  }}
                />
              </div>
            );
          })}
      </div>
    </div>
  );
};

const numberOfLines = (content: string) => {
  return content.split("\n").length;
};

const styles = {
  files: {
    display: "flex",
    flexDirection: "column",
    gap: 16,
  },

  filePanel: (theme) => ({
    borderRadius: 8,
    border: `1px solid ${theme.palette.divider}`,
  }),

  fileHeader: (theme) => ({
    padding: "8px 16px",
    borderBottom: `1px solid ${theme.palette.divider}`,
    fontSize: 13,
    fontWeight: 500,
    display: "flex",
    gap: 8,
    alignItems: "center",
  }),
} satisfies Record<string, Interpolation<Theme>>;
