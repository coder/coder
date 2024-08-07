import type { FC } from "react";
import type { Organization } from "api/typesGenerated";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";

interface OrganizationSummaryPageViewProps {
  organization: Organization;
}

export const OrganizationSummaryPageView: FC<
  OrganizationSummaryPageViewProps
> = (props) => {
  return (
    <div>
      <PageHeader
        css={{
          // The deployment settings layout already has padding.
          paddingTop: 0,
        }}
      >
        <Stack direction="row" spacing={3} alignItems="center">
          <UserAvatar
            key={props.organization.id}
            size="xl"
            username={
              props.organization.display_name || props.organization.name
            }
            avatarURL={props.organization.icon}
          />
          <div>
            <PageHeaderTitle>
              {props.organization.display_name || props.organization.name}
            </PageHeaderTitle>
            {props.organization.description && (
              <PageHeaderSubtitle>
                {props.organization.description}
              </PageHeaderSubtitle>
            )}
          </div>
        </Stack>
      </PageHeader>
      You are a member of this organization.
    </div>
  );
};
