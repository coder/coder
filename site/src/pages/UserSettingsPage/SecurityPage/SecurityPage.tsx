import { type ComponentProps, type FC } from "react";
import { useMutation, useQuery } from "react-query";
import { getUserLoginType } from "api/api";
import { authMethods, updatePassword } from "api/queries/users";
import { useMe } from "contexts/auth/useMe";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Section } from "../Section";
import { SecurityForm } from "./SecurityForm";
import {
  SingleSignOnSection,
  useSingleSignOnSection,
} from "./SingleSignOnSection";

export const SecurityPage: FC = () => {
  const me = useMe();
  const updatePasswordMutation = useMutation(updatePassword());
  const authMethodsQuery = useQuery(authMethods());
  const { data: userLoginType } = useQuery({
    queryKey: ["loginType"],
    queryFn: getUserLoginType,
  });
  const singleSignOnSection = useSingleSignOnSection();

  if (!authMethodsQuery.data || !userLoginType) {
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
          authMethods: authMethodsQuery.data,
          userLoginType,
          ...singleSignOnSection,
        },
      }}
    />
  );
};

interface SecurityPageViewProps {
  security: {
    form: ComponentProps<typeof SecurityForm>;
  };
  oidc?: {
    section: ComponentProps<typeof SingleSignOnSection>;
  };
}

export const SecurityPageView: FC<SecurityPageViewProps> = ({
  security,
  oidc,
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
