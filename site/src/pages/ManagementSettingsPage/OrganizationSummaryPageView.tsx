import { AvatarFallback, AvatarImage } from "@radix-ui/react-avatar";
import type { Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "modules/users/UserAvatar/UserAvatar";
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
				<Stack direction="row">
					<Avatar key={organization.id} size="lg" variant="icon">
						<AvatarImage src={organization.icon} />
						<AvatarFallback>
							{organization.display_name || organization.name}
						</AvatarFallback>
					</Avatar>
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
