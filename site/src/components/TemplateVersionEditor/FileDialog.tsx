import TextField from "@material-ui/core/TextField"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Stack } from "components/Stack/Stack"
import { ChangeEvent, FC, useState } from "react"
import Typography from "@material-ui/core/Typography"

export const CreateFileDialog: FC<{
  onClose: () => void
  checkExists: (path: string) => boolean
  onConfirm: (path: string) => void
  open: boolean
}> = ({ checkExists, onClose, onConfirm, open }) => {
  const [pathValue, setPathValue] = useState("")
  const [error, setError] = useState("")
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    setPathValue(event.target.value)
  }
  const handleConfirm = () => {
    if (pathValue === "") {
      setError("You must enter a path!")
      return
    }
    if (checkExists(pathValue)) {
      setError("File already exists")
      return
    }
    onConfirm(pathValue)
    setPathValue("")
  }

  return (
    <ConfirmDialog
      open={open}
      onClose={() => {
        onClose()
        setPathValue("")
      }}
      onConfirm={handleConfirm}
      hideCancel={false}
      type="success"
      cancelText="Cancel"
      confirmText="Create"
      title="Create File"
      description={
        <Stack spacing={1}>
          <Typography>
            Specify the path to a file to be created. This path can contain
            slashes too!
          </Typography>
          <TextField
            autoFocus
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                handleConfirm()
              }
            }}
            helperText={error}
            name="file-path"
            autoComplete="off"
            id="file-path"
            placeholder="main.tf"
            value={pathValue}
            onChange={handleChange}
            label="File Path"
          />
        </Stack>
      }
    />
  )
}

export const DeleteFileDialog: FC<{
  onClose: () => void
  onConfirm: () => void
  open: boolean
  filename: string
}> = ({ onClose, onConfirm, open, filename }) => {
  return (
    <ConfirmDialog
      type="delete"
      onClose={onClose}
      open={open}
      onConfirm={onConfirm}
      title="Delete File"
      description={`Are you sure you want to delete "${filename}"?`}
    />
  )
}

export const RenameFileDialog: FC<{
  onClose: () => void
  onConfirm: (filename: string) => void
  checkExists: (path: string) => boolean
  open: boolean
  filename: string
}> = ({ checkExists, onClose, onConfirm, open, filename }) => {
  const [pathValue, setPathValue] = useState(filename)
  const [error, setError] = useState("")
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    setPathValue(event.target.value)
  }
  const handleConfirm = () => {
    if (pathValue === "") {
      setError("You must enter a path!")
      return
    }
    if (checkExists(pathValue)) {
      setError("File already exists")
      return
    }
    onConfirm(pathValue)
    setPathValue("")
  }

  return (
    <ConfirmDialog
      open={open}
      onClose={() => {
        onClose()
        setPathValue("")
      }}
      onConfirm={handleConfirm}
      hideCancel={false}
      type="success"
      cancelText="Cancel"
      confirmText="Create"
      title="Rename File"
      description={
        <Stack spacing={1}>
          <Typography>
            Rename {`"${filename}"`} to something else. This path can contain
            slashes too!
          </Typography>
          <TextField
            autoFocus
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                handleConfirm()
              }
            }}
            helperText={error}
            name="file-path"
            autoComplete="off"
            id="file-path"
            placeholder="main.tf"
            defaultValue={filename}
            value={pathValue}
            onChange={handleChange}
            label="File Path"
          />
        </Stack>
      }
    />
  )
}
