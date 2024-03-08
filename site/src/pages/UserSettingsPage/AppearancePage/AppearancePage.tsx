import CircularProgress from "@mui/material/CircularProgress";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { updateAppearanceSettings } from "api/queries/users";
import { Stack } from "components/Stack/Stack";
import { useMe } from "contexts/auth/useMe";
import { Section } from "../Section";
import { AppearanceForm } from "./AppearanceForm";

export const AppearancePage: FC = () => {
  const me = useMe();
  const queryClient = useQueryClient();
  const updateAppearanceSettingsMutation = useMutation(
    updateAppearanceSettings("me", queryClient),
  );

  return (
    <>
      <Section
        title={
          <Stack direction="row" alignItems="center">
            <span>Theme</span>
            {updateAppearanceSettingsMutation.isLoading && (
              <CircularProgress size={16} />
            )}
          </Stack>
        }
        layout="fluid"
      >
        <AppearanceForm
          isUpdating={updateAppearanceSettingsMutation.isLoading}
          error={updateAppearanceSettingsMutation.error}
          initialValues={{ theme_preference: me.theme_preference }}
          onSubmit={updateAppearanceSettingsMutation.mutateAsync}
        />
      </Section>
    </>
  );
};

export default AppearancePage;
