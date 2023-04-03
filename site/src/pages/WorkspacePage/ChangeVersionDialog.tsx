import { DialogProps } from "components/Dialogs/Dialog"
import { FC, useRef, useState } from "react"
import { FormFields } from "components/Form/Form"
import TextField from "@material-ui/core/TextField"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Stack } from "components/Stack/Stack"
import { Template, TemplateVersion } from "api/typesGenerated"
import { Loader } from "components/Loader/Loader"
import Autocomplete from "@material-ui/lab/Autocomplete"
import { createDayString } from "util/createDayString"
import { AvatarData } from "components/AvatarData/AvatarData"
import { Pill } from "components/Pill/Pill"
import { Avatar } from "components/Avatar/Avatar"
import CircularProgress from "@material-ui/core/CircularProgress"

export type ChangeVersionDialogProps = DialogProps & {
  template: Template | undefined
  templateVersions: TemplateVersion[] | undefined
  defaultTemplateVersion: TemplateVersion | undefined
  onClose: () => void
  onConfirm: (templateVersion: TemplateVersion) => void
}

export const ChangeVersionDialog: FC<ChangeVersionDialogProps> = ({
  onConfirm,
  onClose,
  template,
  templateVersions,
  defaultTemplateVersion,
  ...dialogProps
}) => {
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false)
  const selectedTemplateVersion = useRef<TemplateVersion | undefined>()

  return (
    <ConfirmDialog
      {...dialogProps}
      onClose={onClose}
      onConfirm={() => {
        if (selectedTemplateVersion.current) {
          onConfirm(selectedTemplateVersion.current)
        }
      }}
      hideCancel={false}
      type="success"
      cancelText="Cancel"
      confirmText="Change"
      title="Change version"
      description={
        <Stack>
          <p>You are about to change the version of this workspace.</p>
          {templateVersions ? (
            <FormFields>
              <Autocomplete
                disableClearable
                options={templateVersions}
                defaultValue={defaultTemplateVersion}
                id="template-version-autocomplete"
                open={isAutocompleteOpen}
                onChange={(_, newTemplateVersion) => {
                  selectedTemplateVersion.current =
                    newTemplateVersion ?? undefined
                }}
                onOpen={() => {
                  setIsAutocompleteOpen(true)
                }}
                onClose={() => {
                  setIsAutocompleteOpen(false)
                }}
                getOptionSelected={(
                  option: TemplateVersion,
                  value: TemplateVersion,
                ) => option.id === value.id}
                getOptionLabel={(option) => option.name}
                renderOption={(option: TemplateVersion) => (
                  <AvatarData
                    avatar={
                      <Avatar src={option.created_by.avatar_url}>
                        {option.name}
                      </Avatar>
                    }
                    title={
                      <Stack
                        direction="row"
                        justifyContent="space-between"
                        style={{ width: "100%" }}
                      >
                        {option.name}
                        {template?.active_version_id === option.id && (
                          <Pill text="Active" type="success" />
                        )}
                      </Stack>
                    }
                    subtitle={createDayString(option.created_at)}
                  />
                )}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    fullWidth
                    variant="outlined"
                    placeholder="Template version name"
                    InputProps={{
                      ...params.InputProps,
                      endAdornment: (
                        <>
                          {!templateVersions ? (
                            <CircularProgress size={16} />
                          ) : null}
                          {params.InputProps.endAdornment}
                        </>
                      ),
                    }}
                  />
                )}
              />
            </FormFields>
          ) : (
            <Loader />
          )}
        </Stack>
      }
    />
  )
}
