import type { Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
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
					<Avatar
						size="lg"
						variant="icon"
						src={organization.icon}
						fallback={organization.display_name || organization.name}
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
