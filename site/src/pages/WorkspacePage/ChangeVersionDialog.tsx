import { type FC, useRef, useState } from "react";
import TextField from "@mui/material/TextField";
import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import Box from "@mui/material/Box";
import AlertTitle from "@mui/material/AlertTitle";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";
import type { Template, TemplateVersion } from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import type { DialogProps } from "components/Dialogs/Dialog";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { FormFields } from "components/Form/Form";
import { Stack } from "components/Stack/Stack";
import { Loader } from "components/Loader/Loader";
import { AvatarData } from "components/AvatarData/AvatarData";
import { Pill } from "components/Pill/Pill";
import { Avatar } from "components/Avatar/Avatar";
import { createDayString } from "utils/createDayString";

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
  const theme = useTheme();

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
                            // Same `padding-left` as input
                            root: css`
                              padding-left: ${theme.spacing(1.75)} !important;
                            `,
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
