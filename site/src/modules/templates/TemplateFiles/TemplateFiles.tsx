import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import { useMemo, type FC, useCallback } from "react";
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter";
import { TemplateVersionFiles } from "utils/templateVersion";
import RadioButtonCheckedOutlined from "@mui/icons-material/RadioButtonCheckedOutlined";
import { FileTree } from "utils/filetree";
import set from "lodash/fp/set";
import { TemplateFileTree } from "./TemplateFileTree";
import { Link } from "react-router-dom";
import EditOutlined from "@mui/icons-material/EditOutlined";

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
  versionName: string;
  templateName: string;
}

export const TemplateFiles: FC<TemplateFilesProps> = ({
  currentFiles,
  baseFiles,
  versionName,
  templateName,
}) => {
  const filenames = Object.keys(currentFiles);
  const theme = useTheme();

  const fileInfo = useCallback(
    (filename: string) => {
      const value = currentFiles[filename].trim();
      const previousValue = baseFiles ? baseFiles[filename]?.trim() : undefined;
      const hasDiff = previousValue && value !== previousValue;

      return {
        value,
        previousValue,
        hasDiff,
      };
    },
    [baseFiles, currentFiles],
  );

  const fileTree: FileTree = useMemo(() => {
    let tree: FileTree = {};
    for (const filename of filenames) {
      const info = fileInfo(filename);
      tree = set(filename.split("/"), info.value, tree);
    }
    return tree;
  }, [fileInfo, filenames]);

  return (
    <div>
      <div css={{ display: "flex", alignItems: "flex-start", gap: 32 }}>
        <div css={styles.sidebar}>
          <TemplateFileTree
            fileTree={fileTree}
            onSelect={function (path: string): void {
              window.location.hash = path;
            }}
            Label={({ path, filename, isFolder }) => {
              if (isFolder) {
                return <>{filename}</>;
              }

              const hasDiff = fileInfo(path).hasDiff;
              return (
                <span
                  css={{
                    color: hasDiff
                      ? theme.roles.warning.fill.outline
                      : undefined,
                  }}
                >
                  {filename}
                </span>
              );
            }}
          />
        </div>

        <div css={styles.files} data-testid="template-files-content">
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

                    <div css={{ marginLeft: "auto" }}>
                      <Link
                        to={`/templates/${templateName}/versions/${versionName}/edit?path=${filename}`}
                        css={{
                          display: "flex",
                          gap: 4,
                          alignItems: "center",
                          fontSize: 14,
                          color: theme.palette.text.secondary,
                          textDecoration: "none",

                          "&:hover": {
                            color: theme.palette.text.primary,
                          },
                        }}
                      >
                        <EditOutlined css={{ fontSize: "inherit" }} />
                        Edit
                      </Link>
                    </div>
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
    </div>
  );
};

const numberOfLines = (content: string) => {
  return content.split("\n").length;
};

const styles = {
  sidebar: (theme) => ({
    width: 240,
    flexShrink: 0,
    borderRadius: 8,
    overflow: "auto",
    border: `1px solid ${theme.palette.divider}`,
    padding: "4px 0",
    position: "sticky",
    top: 32,
  }),

  files: {
    display: "flex",
    flexDirection: "column",
    gap: 16,
    flex: 1,
  },

  filePanel: (theme) => ({
    borderRadius: 8,
    border: `1px solid ${theme.palette.divider}`,
    overflow: "hidden",
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
