import { makeStyles } from "@mui/styles";
import { Stack } from "components/Stack/Stack";
import { FC, DragEvent, useRef, ReactNode } from "react";
import UploadIcon from "@mui/icons-material/CloudUploadOutlined";
import { useClickable } from "hooks/useClickable";
import CircularProgress from "@mui/material/CircularProgress";
import { combineClasses } from "utils/combineClasses";
import IconButton from "@mui/material/IconButton";
import RemoveIcon from "@mui/icons-material/DeleteOutline";
import FileIcon from "@mui/icons-material/FolderOutlined";

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
  const styles = useStyles();
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
        className={styles.file}
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
        className={combineClasses({
          [styles.root]: true,
          [styles.disabled]: isUploading,
        })}
        {...clickable}
        {...tarDrop}
      >
        <Stack alignItems="center" spacing={1}>
          {isUploading ? (
            <CircularProgress size={32} />
          ) : (
            <UploadIcon className={styles.icon} />
          )}

          <Stack alignItems="center" spacing={0.5}>
            <span className={styles.title}>{title}</span>
            <span className={styles.description}>{description}</span>
          </Stack>
        </Stack>
      </div>

      <input
        type="file"
        data-testid="file-upload"
        ref={inputRef}
        className={styles.input}
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

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.shape.borderRadius,
    border: `2px dashed ${theme.palette.divider}`,
    padding: theme.spacing(6),
    cursor: "pointer",

    "&:hover": {
      backgroundColor: theme.palette.background.paper,
    },
  },

  disabled: {
    pointerEvents: "none",
    opacity: 0.75,
  },

  icon: {
    fontSize: theme.spacing(8),
  },

  title: {
    fontSize: theme.spacing(2),
  },

  description: {
    color: theme.palette.text.secondary,
    textAlign: "center",
    maxWidth: theme.spacing(50),
  },

  input: {
    display: "none",
  },

  file: {
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    padding: theme.spacing(2),
    background: theme.palette.background.paper,
  },
}));
