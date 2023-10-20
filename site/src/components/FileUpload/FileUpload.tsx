import { Stack } from "components/Stack/Stack";
import { type FC, type DragEvent, useRef, type ReactNode } from "react";
import UploadIcon from "@mui/icons-material/CloudUploadOutlined";
import { useClickable } from "hooks/useClickable";
import CircularProgress from "@mui/material/CircularProgress";
import IconButton from "@mui/material/IconButton";
import RemoveIcon from "@mui/icons-material/DeleteOutline";
import FileIcon from "@mui/icons-material/FolderOutlined";
import { css, type Interpolation, type Theme } from "@emotion/react";

const useFileDrop = (
  callback: (file: File) => void,
  fileTypeRequired?: string,
): {
  onDragOver: (e: DragEvent<HTMLDivElement>) => void;
  onDrop: (e: DragEvent<HTMLDivElement>) => void;
} => {
  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
  };

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    const file = e.dataTransfer.files[0];
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- file can be undefined
    if (!file) {
      return;
    }
    if (fileTypeRequired && file.type !== fileTypeRequired) {
      return;
    }
    callback(file);
  };

  return {
    onDragOver,
    onDrop,
  };
};

export interface FileUploadProps {
  isUploading: boolean;
  onUpload: (file: File) => void;
  onRemove?: () => void;
  file?: File;
  removeLabel: string;
  title: string;
  description?: ReactNode;
  extension?: string;
  fileTypeRequired?: string;
}

export const FileUpload: FC<FileUploadProps> = ({
  isUploading,
  onUpload,
  onRemove,
  file,
  removeLabel,
  title,
  description,
  extension,
  fileTypeRequired,
}) => {
  const inputRef = useRef<HTMLInputElement>(null);
  const tarDrop = useFileDrop(onUpload, fileTypeRequired);

  const clickable = useClickable<HTMLDivElement>(() => {
    if (inputRef.current) {
      inputRef.current.click();
    }
  });

  if (!isUploading && file) {
    return (
      <Stack
        css={styles.file}
        direction="row"
        justifyContent="space-between"
        alignItems="center"
      >
        <Stack direction="row" alignItems="center">
          <FileIcon />
          <span>{file.name}</span>
        </Stack>

        <IconButton title={removeLabel} size="small" onClick={onRemove}>
          <RemoveIcon />
        </IconButton>
      </Stack>
    );
  }

  return (
    <>
      <div
        css={[styles.root, isUploading && styles.disabled]}
        {...clickable}
        {...tarDrop}
      >
        <Stack alignItems="center" spacing={1}>
          {isUploading ? (
            <CircularProgress size={32} />
          ) : (
            <UploadIcon css={styles.icon} />
          )}

          <Stack alignItems="center" spacing={0.5}>
            <span css={styles.title}>{title}</span>
            <span css={styles.description}>{description}</span>
          </Stack>
        </Stack>
      </div>

      <input
        type="file"
        data-testid="file-upload"
        ref={inputRef}
        css={styles.input}
        accept={extension}
        onChange={(event) => {
          const file = event.currentTarget.files?.[0];
          if (file) {
            onUpload(file);
          }
        }}
      />
    </>
  );
};

const styles = {
  root: (theme) => css`
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: ${theme.shape.borderRadius}px;
    border: 2px dashed ${theme.palette.divider};
    padding: ${theme.spacing(6)};
    cursor: pointer;

    &:hover {
      background-color: ${theme.palette.background.paper};
    }
  `,

  disabled: {
    pointerEvents: "none",
    opacity: 0.75,
  },

  icon: (theme) => ({
    fontSize: theme.spacing(8),
  }),

  title: (theme) => ({
    fontSize: theme.spacing(2),
  }),

  description: (theme) => ({
    color: theme.palette.text.secondary,
    textAlign: "center",
    maxWidth: theme.spacing(50),
  }),

  input: {
    display: "none",
  },

  file: (theme) => ({
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    padding: theme.spacing(2),
    background: theme.palette.background.paper,
  }),
} satisfies Record<string, Interpolation<Theme>>;
