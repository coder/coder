import { type FC } from "react";
import { Section } from "components/SettingsLayout/Section";
import { AccountForm } from "./AccountForm";
import { useAuth } from "components/AuthProvider/AuthProvider";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";
import { Stack } from "@mui/system";
import { useQuery } from "react-query";
import { groupsForUser } from "api/queries/groups";
import { useOrganizationId } from "hooks";
import { useTheme } from "@emotion/react";
import { Loader } from "components/Loader/Loader";
import { Group } from "api/typesGenerated";

export const AccountPage: FC = () => {
  const theme = useTheme();
  const { updateProfile, updateProfileError, isUpdatingProfile } = useAuth();
  const permissions = usePermissions();

  const me = useMe();
  const organizationId = useOrganizationId();
  const groupsQuery = useQuery(groupsForUser(organizationId, me.id));

  return (
    <Stack spacing={6}>
      <Section title="Account" description="Update your account info">
        <AccountForm
          editable={permissions?.updateUsers ?? false}
          email={me.email}
          updateProfileError={updateProfileError}
          isLoading={isUpdatingProfile}
          initialValues={{ username: me.username }}
          onSubmit={updateProfile}
        />
      </Section>

      <Section
        title="Your groups"
        layout="fluid"
        description={
          groupsQuery.isSuccess && (
            <span>
              You are in{" "}
              <em
                css={{
                  fontStyle: "normal",
                  color: theme.palette.text.primary,
                  fontWeight: 600,
                }}
              >
                {groupsQuery.data.length} groups
              </em>
            </span>
          )
        }
      >
        {groupsQuery.isSuccess ? (
          <GroupList groups={groupsQuery.data} />
        ) : (
          <Loader />
        )}
      </Section>
    </Stack>
  );
};

type GroupListProps = {
  groups: readonly Group[];
};

function GroupList({ groups }: GroupListProps) {
  const theme = useTheme();

  return (
    <>
      {groups.map((group) => (
        <div key={group.id} css={{ backgroundColor: "blue" }}>
          <span>{group.display_name || group.name}</span>
          <span>{group.members.length} members</span>
        </div>
      ))}
    </>
  );
}

export default AccountPage;
