import { type FC } from "react";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";
import { useQuery } from "react-query";
import { groupsForUser } from "api/queries/groups";
import { useOrganizationId } from "hooks";
import { useTheme } from "@emotion/react";

import { Stack } from "@mui/system";
import Grid from "@mui/material/Grid";

import { AccountForm } from "./AccountForm";
import { useAuth } from "components/AuthProvider/AuthProvider";
import { Section } from "components/SettingsLayout/Section";
import { Loader } from "components/Loader/Loader";
import { AvatarCard } from "components/AvatarCard/AvatarCard";
import { ErrorAlert } from "components/Alert/ErrorAlert";

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
        <div
          css={{ display: "flex", flexFlow: "column nowrap", rowGap: "24px" }}
        >
          {groupsQuery.isError && <ErrorAlert error={groupsQuery.error} />}

          <Grid container columns={{ xs: 1, md: 2 }} spacing="16px">
            {groupsQuery.data?.map((group) => (
              <Grid item key={group.id} xs={1}>
                <AvatarCard
                  imgUrl={group.avatar_url}
                  altText={group.display_name || group.name}
                  header={group.display_name || group.name}
                  subtitle={
                    <>
                      {group.members.length} member
                      {group.members.length !== 1 && "s"}
                    </>
                  }
                />
              </Grid>
            ))}
          </Grid>

          {groupsQuery.isLoading && <Loader />}
        </div>
      </Section>
    </Stack>
  );
};

export default AccountPage;
