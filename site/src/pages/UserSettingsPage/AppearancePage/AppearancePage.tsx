import CircularProgress from "@mui/material/CircularProgress";
import { type FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { updateThemePreference } from "api/queries/users";
import { Stack } from "components/Stack/Stack";
import { useMe } from "hooks";
import { Section } from "../Section";
import { AppearanceForm } from "./AppearanceForm";

export const AppearancePage: FC = () => {
  const me = useMe();
  const queryClient = useQueryClient();
  const updateThemePreferenceMutation = useMutation(
    updateThemePreference("me", queryClient),
  );

  return (
    <>
      <Section
        title={
          <Stack direction="row" alignItems="center">
            <span>Theme</span>
            {updateThemePreferenceMutation.isLoading && (
              <CircularProgress size={16} />
            )}
          </Stack>
        }
        layout="fluid"
      >
        <AppearanceForm
          isUpdating={updateThemePreferenceMutation.isLoading}
          error={updateThemePreferenceMutation.error}
          initialValues={{ theme_preference: me.theme_preference }}
          onSubmit={updateThemePreferenceMutation.mutateAsync}
        />
      </Section>
    </>
  );
};

export default AppearancePage;
