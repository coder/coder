import { makeStyles } from "@material-ui/core/styles"
import { Stack } from "components/Stack/Stack"
import { FC, DragEvent, useRef } from "react"
import UploadIcon from "@material-ui/icons/CloudUploadOutlined"
import { useClickable } from "hooks/useClickable"
import CircularProgress from "@material-ui/core/CircularProgress"
import { combineClasses } from "util/combineClasses"
import IconButton from "@material-ui/core/IconButton"
import RemoveIcon from "@material-ui/icons/DeleteOutline"
import FileIcon from "@material-ui/icons/FolderOutlined"

const useTarDrop = (
  callback: (file: File) => void,
): {
  onDragOver: (e: DragEvent<HTMLDivElement>) => void
  onDrop: (e: DragEvent<HTMLDivElement>) => void
} => {
  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
  }

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    const file = e.dataTransfer.files[0]
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- file can be undefined
    if (!file || file.type !== "application/x-tar") {
      return
    }
    callback(file)
  }

  return {
    onDragOver,
    onDrop,
  }
}

export interface TemplateUploadProps {
  isUploading: boolean
  onUpload: (file: File) => void
  onRemove: () => void
  file?: File
}

export const TemplateUpload: FC<TemplateUploadProps> = ({
  isUploading,
  onUpload,
  onRemove,
  file,
}) => {
  const styles = useStyles()
  const inputRef = useRef<HTMLInputElement>(null)
  const tarDrop = useTarDrop(onUpload)
  const clickable = useClickable(() => {
    if (inputRef.current) {
      inputRef.current.click()
    }
  })

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

        <IconButton title="Remove file" size="small" onClick={onRemove}>
          <RemoveIcon />
        </IconButton>
      </Stack>
    )
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
            <span className={styles.title}>Upload template</span>
            <span className={styles.description}>
              The template needs to be in a .tar file
            </span>
          </Stack>
        </Stack>
      </div>

      <input
        type="file"
        ref={inputRef}
        className={styles.input}
        accept=".tar"
        onChange={(event) => {
          const file = event.currentTarget.files?.[0]
          if (file) {
            onUpload(file)
          }
        }}
      />
    </>
  )
}

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
}))
