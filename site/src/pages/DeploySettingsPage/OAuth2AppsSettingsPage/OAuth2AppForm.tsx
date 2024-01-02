import LoadingButton from "@mui/lab/LoadingButton";
import TextField from "@mui/material/TextField";
import { type FC, type ReactNode } from "react";
import { isApiValidationError, mapApiErrorToFieldErrors } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";

type OAuth2AppFormProps = {
  app?: TypesGen.OAuth2ProviderApp;
  onSubmit: (data: {
    name: string;
    callback_url: string;
    icon: string;
  }) => void;
  error?: unknown;
  isUpdating: boolean;
  actions?: ReactNode;
};

export const OAuth2AppForm: FC<OAuth2AppFormProps> = ({
  app,
  onSubmit,
  error,
  isUpdating,
  actions,
}) => {
  const apiValidationErrors = isApiValidationError(error)
    ? mapApiErrorToFieldErrors(error.response.data)
    : undefined;

  return (
    <form
      css={{ marginTop: 10 }}
      onSubmit={(event) => {
        event.preventDefault();
        const formData = new FormData(event.target as HTMLFormElement);
        onSubmit({
          name: formData.get("name") as string,
          callback_url: formData.get("callback_url") as string,
          icon: formData.get("icon") as string,
        });
      }}
    >
      <Stack spacing={2.5}>
        <TextField
          name="name"
          label="Application name"
          defaultValue={app?.name}
          error={Boolean(apiValidationErrors?.name)}
          helperText={
            apiValidationErrors?.name || "The name of your Coder app."
          }
          autoFocus
          fullWidth
        />
        <TextField
          name="callback_url"
          label="Callback URL"
          defaultValue={app?.callback_url}
          error={Boolean(apiValidationErrors?.callback_url)}
          helperText={
            apiValidationErrors?.callback_url ||
            "The full URL to redirect to after a user authorizes an installation."
          }
          fullWidth
        />
        <TextField
          name="icon"
          label="Application icon"
          defaultValue={app?.icon}
          error={Boolean(apiValidationErrors?.icon)}
          helperText={
            apiValidationErrors?.icon || "A full or relative URL to an icon."
          }
          fullWidth
        />

        <Stack direction="row">
          <LoadingButton loading={isUpdating} type="submit" variant="contained">
            {app ? "Update application" : "Create application"}
          </LoadingButton>
          {actions}
        </Stack>
      </Stack>
    </form>
  );
};
