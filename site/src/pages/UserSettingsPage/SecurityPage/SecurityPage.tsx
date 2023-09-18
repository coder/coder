import { useMe } from "hooks/useMe";
import { ComponentProps, FC } from "react";
import { Section } from "components/SettingsLayout/Section";
import { SecurityForm } from "./SettingsSecurityForm";
import { useMutation, useQuery } from "@tanstack/react-query";
import { getAuthMethods, getUserLoginType } from "api/api";
import {
  SingleSignOnSection,
  useSingleSignOnSection,
} from "./SingleSignOnSection";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { updatePassword } from "api/queries/users";
import { displaySuccess } from "components/GlobalSnackbar/utils";

export const SecurityPage: FC = () => {
  const me = useMe();
  const updatePasswordMutation = useMutation(updatePassword());
  const { data: authMethods } = useQuery({
    queryKey: ["authMethods"],
    queryFn: getAuthMethods,
  });
  const { data: userLoginType } = useQuery({
    queryKey: ["loginType"],
    queryFn: getUserLoginType,
  });
  const singleSignOnSection = useSingleSignOnSection();

  if (!authMethods || !userLoginType) {
    return <Loader />;
  }

  return (
    <SecurityPageView
      security={{
        form: {
          disabled: userLoginType.login_type !== "password",
          error: updatePasswordMutation.error,
          isLoading: updatePasswordMutation.isLoading,
          onSubmit: async (data) => {
            await updatePasswordMutation.mutateAsync({
              userId: me.id,
              ...data,
            });
            displaySuccess("Updated password.");
            // Refresh the browser session. We need to improve the AuthProvider
            // to include better API to handle these scenarios
            window.location.href = location.origin;
          },
        },
      }}
      oidc={{
        section: {
          authMethods,
          userLoginType,
          ...singleSignOnSection,
        },
      }}
    />
  );
};

export const SecurityPageView = ({
  security,
  oidc,
}: {
  security: {
    form: ComponentProps<typeof SecurityForm>;
  };
  oidc?: {
    section: ComponentProps<typeof SingleSignOnSection>;
  };
}) => {
  return (
    <Stack spacing={6}>
      <Section title="Security" description="Update your account password">
        <SecurityForm {...security.form} />
      </Section>
      {oidc && <SingleSignOnSection {...oidc.section} />}
    </Stack>
  );
};

export default SecurityPage;
