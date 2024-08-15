import type { Organization } from "api/typesGenerated";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import type { FC } from "react";

interface OrganizationSummaryPageViewProps {
	organization: Organization;
}

export const OrganizationSummaryPageView: FC<
	OrganizationSummaryPageViewProps
> = ({ organization }) => {
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
						key={organization.id}
						size="xl"
						username={organization.display_name || organization.name}
						avatarURL={organization.icon}
					/>
					<div>
						<PageHeaderTitle>
							{organization.display_name || organization.name}
						</PageHeaderTitle>
						{organization.description && (
							<PageHeaderSubtitle>
								{organization.description}
							</PageHeaderSubtitle>
						)}
					</div>
				</Stack>
			</PageHeader>
			You are a member of this organization.
		</div>
	);
};
