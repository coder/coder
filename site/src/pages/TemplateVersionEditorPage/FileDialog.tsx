import TextField from "@mui/material/TextField";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Stack } from "components/Stack/Stack";
import { type ChangeEvent, type FC, useState } from "react";
import { allowedExtensions, isAllowedFile } from "utils/templateVersion";
import { type FileTree, isFolder, validatePath } from "utils/filetree";

interface CreateFileDialogProps {
  onClose: () => void;
  checkExists: (path: string) => boolean;
  onConfirm: (path: string) => void;
  open: boolean;
  fileTree: FileTree;
}

export const CreateFileDialog: FC<CreateFileDialogProps> = ({
  checkExists,
  onClose,
  onConfirm,
  open,
  fileTree,
}) => {
  const [pathValue, setPathValue] = useState("");
  const [error, setError] = useState<string>();
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    setPathValue(event.target.value);
  };
  const handleConfirm = () => {
    if (pathValue === "") {
      setError("You must enter a path!");
      return;
    }
    if (checkExists(pathValue)) {
      setError("File already exists");
      return;
    }
    if (!isAllowedFile(pathValue)) {
      const extensions = allowedExtensions.join(", ");
      setError(
        `This extension is not allowed. You only can create files with the following extensions: ${extensions}.`,
      );
      return;
    }
    const pathError = validatePath(pathValue, fileTree);
    if (pathError) {
      setError(pathError);
      return;
    }
    onConfirm(pathValue);
    setError(undefined);
    setPathValue("");
  };

  return (
    <ConfirmDialog
      open={open}
      onClose={() => {
        onClose();
        setError(undefined);
        setPathValue("");
      }}
      onConfirm={handleConfirm}
      hideCancel={false}
      type="success"
      cancelText="Cancel"
      confirmText="Create"
      title="Create File"
      description={
        <Stack spacing={4}>
          <p>
            Specify the path to a file to be created. This path can contain
            slashes too.
          </p>
          <TextField
            autoFocus
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                handleConfirm();
              }
            }}
            error={Boolean(error)}
            helperText={error}
            name="file-path"
            autoComplete="off"
            id="file-path"
            placeholder="example.tf"
            value={pathValue}
            onChange={handleChange}
            label="File Path"
          />
        </Stack>
      }
    />
  );
};

interface DeleteFileDialogProps {
  onClose: () => void;
  onConfirm: () => void;
  open: boolean;
  filename: string;
}

export const DeleteFileDialog: FC<DeleteFileDialogProps> = ({
  onClose,
  onConfirm,
  open,
  filename,
}) => {
  return (
    <ConfirmDialog
      type="delete"
      onClose={onClose}
      open={open}
      onConfirm={onConfirm}
      title="Delete File"
      description={
        <>
          Are you sure you want to delete <strong>{filename}</strong>? It will
          be deleted permanently.
        </>
      }
    />
  );
};

interface RenameFileDialogProps {
  onClose: () => void;
  onConfirm: (filename: string) => void;
  checkExists: (path: string) => boolean;
  open: boolean;
  filename: string;
  fileTree: FileTree;
}

export const RenameFileDialog: FC<RenameFileDialogProps> = ({
  checkExists,
  onClose,
  onConfirm,
  open,
  filename,
  fileTree,
}) => {
  const [pathValue, setPathValue] = useState(filename);
  const [error, setError] = useState<string>();
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    setPathValue(event.target.value);
  };
  const handleConfirm = () => {
    if (pathValue === "") {
      setError("You must enter a path!");
      return;
    }
    if (checkExists(pathValue)) {
      setError("File already exists");
      return;
    }
    if (!isAllowedFile(pathValue)) {
      const extensions = allowedExtensions.join(", ");
      setError(
        `This extension is not allowed. You only can rename files with the following extensions: ${extensions}.`,
      );
      return;
    }
    //Check if a folder is renamed to a file
    const [_, extension] = pathValue.split(".");
    if (isFolder(filename, fileTree) && extension) {
      setError(`A folder can't be renamed to a file.`);
      return;
    }
    const pathError = validatePath(pathValue, fileTree);
    if (pathError) {
      setError(pathError);
      return;
    }
    onConfirm(pathValue);
    setError(undefined);
    setPathValue("");
  };

  return (
    <ConfirmDialog
      open={open}
      onClose={() => {
        onClose();
        setError(undefined);
        setPathValue("");
      }}
      onConfirm={handleConfirm}
      hideCancel={false}
      type="success"
      cancelText="Cancel"
      confirmText="Rename"
      title="Rename File"
      description={
        <Stack>
          <p>
            Rename <strong>{filename}</strong> to something else. This path can
            contain slashes too!
          </p>
          <TextField
            autoFocus
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                handleConfirm();
              }
            }}
            error={Boolean(error)}
            helperText={error}
            name="file-path"
            autoComplete="off"
            id="file-path"
            placeholder={filename}
            value={pathValue}
            onChange={handleChange}
            label="File Path"
          />
        </Stack>
      }
    />
  );
};
