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
            Specify the path to a file to be created. This path can contain slashes too!
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
