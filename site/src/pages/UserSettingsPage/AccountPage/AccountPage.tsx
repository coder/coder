import { FC } from "react";
import { Section } from "components/SettingsLayout/Section";
import { AccountForm } from "./AccountForm";
import { useAuth } from "components/AuthProvider/AuthProvider";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";

export const AccountPage: FC = () => {
  const { updateProfile, actor } = useAuth();
  const [authState] = actor;
  const me = useMe();
  const permissions = usePermissions();
  const { updateProfileError } = authState.context;
  const canEditUsers = permissions && permissions.updateUsers;

  return (
    <Section title="Account" description="Update your account info">
      <AccountForm
        editable={Boolean(canEditUsers)}
        email={me.email}
        updateProfileError={updateProfileError}
        isLoading={authState.matches("signedIn.profile.updatingProfile")}
        initialValues={{
          username: me.username,
        }}
        onSubmit={updateProfile}
      />
    </Section>
  );
};

export default AccountPage;
