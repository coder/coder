import { type FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { updateThemePreference } from "api/queries/users";
import { Section } from "../Section";
import { AppearanceForm } from "./AppearanceForm";
import { useMe } from "hooks";

export const AppearancePage: FC = () => {
  const me = useMe();
  const queryClient = useQueryClient();
  const updateThemePreferenceMutation = useMutation(
    updateThemePreference("me", queryClient),
  );

  return (
    <>
      <Section title="Theme">
        <AppearanceForm
          isLoading={updateThemePreferenceMutation.isLoading}
          error={updateThemePreferenceMutation.error}
          initialValues={{ theme_preference: me.theme_preference }}
          onSubmit={async (arg: any) => {
            console.log("going");
            const x = await updateThemePreferenceMutation.mutateAsync(arg);
            console.log(x);
            return x;
          }}
        />
      </Section>
    </>
  );
};

export default AppearancePage;
