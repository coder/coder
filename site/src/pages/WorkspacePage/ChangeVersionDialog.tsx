import { DialogProps } from "components/Dialogs/Dialog";
import { FC, useRef, useState } from "react";
import { FormFields } from "components/Form/Form";
import TextField from "@mui/material/TextField";
import { makeStyles } from "@mui/styles";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Stack } from "components/Stack/Stack";
import { Template, TemplateVersion } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import Autocomplete from "@mui/material/Autocomplete";
import { createDayString } from "utils/createDayString";
import { AvatarData } from "components/AvatarData/AvatarData";
import { Pill } from "components/Pill/Pill";
import { Avatar } from "components/Avatar/Avatar";
import CircularProgress from "@mui/material/CircularProgress";
import Box from "@mui/material/Box";
import { Alert, AlertDetail } from "components/Alert/Alert";
import AlertTitle from "@mui/material/AlertTitle";
import InfoIcon from "@mui/icons-material/InfoOutlined";

export type ChangeVersionDialogProps = DialogProps & {
  template: Template | undefined;
  templateVersions: TemplateVersion[] | undefined;
  defaultTemplateVersion: TemplateVersion | undefined;
  onClose: () => void;
  onConfirm: (templateVersion: TemplateVersion) => void;
};

export const ChangeVersionDialog: FC<ChangeVersionDialogProps> = ({
  onConfirm,
  onClose,
  template,
  templateVersions,
  defaultTemplateVersion,
  ...dialogProps
}) => {
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false);
  const selectedTemplateVersion = useRef<TemplateVersion | undefined>();
  const version = selectedTemplateVersion.current;

  const styles = useStyles();

  return (
    <ConfirmDialog
      {...dialogProps}
      onClose={onClose}
      onConfirm={() => {
        if (selectedTemplateVersion.current) {
          onConfirm(selectedTemplateVersion.current);
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
            <>
              <FormFields>
                <Autocomplete
                  disableClearable
                  options={templateVersions}
                  defaultValue={defaultTemplateVersion}
                  id="template-version-autocomplete"
                  open={isAutocompleteOpen}
                  onChange={(_, newTemplateVersion) => {
                    selectedTemplateVersion.current =
                      newTemplateVersion ?? undefined;
                  }}
                  onOpen={() => {
                    setIsAutocompleteOpen(true);
                  }}
                  onClose={() => {
                    setIsAutocompleteOpen(false);
                  }}
                  isOptionEqualToValue={(
                    option: TemplateVersion,
                    value: TemplateVersion,
                  ) => option.id === value.id}
                  getOptionLabel={(option) => option.name}
                  renderOption={(props, option: TemplateVersion) => (
                    <Box component="li" {...props}>
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
                            <Stack
                              direction="row"
                              alignItems="center"
                              spacing={1}
                            >
                              {option.name}
                              {option.message && (
                                <InfoIcon
                                  sx={(theme) => ({
                                    width: theme.spacing(1.5),
                                    height: theme.spacing(1.5),
                                  })}
                                />
                              )}
                            </Stack>
                            {template?.active_version_id === option.id && (
                              <Pill text="Active" type="success" />
                            )}
                          </Stack>
                        }
                        subtitle={createDayString(option.created_at)}
                      />
                    </Box>
                  )}
                  renderInput={(params) => (
                    <>
                      <TextField
                        {...params}
                        fullWidth
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
                          classes: {
                            root: styles.inputRoot,
                          },
                        }}
                      />
                    </>
                  )}
                />
              </FormFields>
              {version && (
                <Alert severity="info">
                  <AlertTitle>
                    Published by {version.created_by.username}
                  </AlertTitle>
                  {version.message && (
                    <AlertDetail>{version.message}</AlertDetail>
                  )}
                </Alert>
              )}
            </>
          ) : (
            <Loader />
          )}
        </Stack>
      }
    />
  );
};

export const useStyles = makeStyles((theme) => ({
  inputRoot: {
    paddingLeft: `${theme.spacing(1.75)} !important`, // Same padding left as input
  },
}));
