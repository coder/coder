import type { Organization, ProvisionerDaemon } from "api/typesGenerated";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { Provisioner } from "modules/provisioners/Provisioner";
import type { FC } from "react";

interface OrganizationProvisionersPageViewProps {
	organization: Organization;
	provisioners: ProvisionerDaemon[];
}

export const OrganizationProvisionersPageView: FC<
	OrganizationProvisionersPageViewProps
> = ({ organization, provisioners }) => {
	return (
		<div>
			<PageHeader
				// The deployment settings layout already has padding.
				css={{ paddingTop: 0 }}
			>
				<PageHeaderTitle>Provisioners</PageHeaderTitle>
			</PageHeader>
			<Stack spacing={4.5}>
				{provisioners.map((provisioner) => (
					<Provisioner key={provisioner.id} provisioner={provisioner} />
				))}
			</Stack>
		</div>
	);
};
