import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import { DockerIcon } from "components/Icons/DockerIcon";
import { MarkdownIcon } from "components/Icons/MarkdownIcon";
import { TerraformIcon } from "components/Icons/TerraformIcon";
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter";
import { UseTabResult } from "hooks/useTab";
import { AllowedExtension, TemplateVersionFiles } from "utils/templateVersion";
import InsertDriveFileOutlined from "@mui/icons-material/InsertDriveFileOutlined";

const iconByExtension: Record<AllowedExtension, JSX.Element> = {
  tf: <TerraformIcon />,
  md: <MarkdownIcon />,
  mkd: <MarkdownIcon />,
  Dockerfile: <DockerIcon />,
  protobuf: <InsertDriveFileOutlined />,
  sh: <InsertDriveFileOutlined />,
  tpl: <InsertDriveFileOutlined />,
};

const getExtension = (filename: string) => {
  if (filename.includes(".")) {
    const [_, extension] = filename.split(".");
    return extension;
  }

  return filename;
};

const languageByExtension: Record<AllowedExtension, string> = {
  tf: "hcl",
  md: "markdown",
  mkd: "markdown",
  Dockerfile: "dockerfile",
  sh: "bash",
  tpl: "tpl",
  protobuf: "protobuf",
};

export const TemplateFiles: FC<{
  currentFiles: TemplateVersionFiles;
  previousFiles?: TemplateVersionFiles;
  tab: UseTabResult;
}> = ({ currentFiles, previousFiles, tab }) => {
  const filenames = Object.keys(currentFiles);
  const selectedFilename = filenames[Number(tab.value)];
  const currentFile = currentFiles[selectedFilename];
  const previousFile = previousFiles && previousFiles[selectedFilename];

  return (
    <div css={styles.files}>
      <div css={styles.tabs}>
        {filenames.map((filename, index) => {
          const tabValue = index.toString();
          const extension = getExtension(filename) as AllowedExtension;
          const icon = iconByExtension[extension];
          const hasDiff =
            previousFiles &&
            previousFiles[filename] &&
            currentFiles[filename] !== previousFiles[filename];

          return (
            <button
              css={[styles.tab, tabValue === tab.value && styles.tabActive]}
              onClick={() => {
                tab.set(tabValue);
              }}
              key={filename}
            >
              {icon}
              {filename}
              {hasDiff && <div css={styles.tabDiff} />}
            </button>
          );
        })}
      </div>

      <SyntaxHighlighter
        value={currentFile}
        compareWith={previousFile}
        language={
          languageByExtension[
            getExtension(selectedFilename) as AllowedExtension
          ]
        }
      />
    </div>
  );
};
const styles = {
  tabs: (theme) => ({
    display: "flex",
    alignItems: "baseline",
    borderBottom: `1px solid ${theme.palette.divider}`,
    gap: 1,
    overflowX: "auto",
  }),

  tab: (theme) => ({
    background: "transparent",
    border: 0,
    padding: "0 24px",
    display: "flex",
    alignItems: "center",
    height: 48,
    opacity: 0.85,
    cursor: "pointer",
    gap: 4,
    position: "relative",
    color: theme.palette.text.secondary,
    whiteSpace: "nowrap",

    "& svg": {
      width: 22,
      maxHeight: 16,
    },

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  }),

  tabActive: (theme) => ({
    opacity: 1,
    background: theme.palette.action.hover,
    color: theme.palette.text.primary,

    "&:after": {
      content: '""',
      display: "block",
      height: 1,
      width: "100%",
      bottom: 0,
      left: 0,
      backgroundColor: theme.palette.primary.main,
      position: "absolute",
    },
  }),

  tabDiff: (theme) => ({
    height: 6,
    width: 6,
    backgroundColor: theme.palette.warning.light,
    borderRadius: "100%",
    marginLeft: 4,
  }),

  codeWrapper: (theme) => ({
    background: theme.palette.background.paper,
  }),

  files: (theme) => ({
    borderRadius: 8,
    border: `1px solid ${theme.palette.divider}`,
  }),

  prism: {
    borderRadius: 0,
  },
} satisfies Record<string, Interpolation<Theme>>;
