import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import type { ProvisionerDaemon } from "api/typesGenerated";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { Provisioner } from "modules/provisioners/Provisioner";
import type { FC } from "react";
import { docs } from "utils/docs";

interface OrganizationProvisionersPageViewProps {
	provisioners: ProvisionerDaemon[];
}

export const OrganizationProvisionersPageView: FC<
	OrganizationProvisionersPageViewProps
> = ({ provisioners }) => {
	return (
		<div>
			<PageHeader
				// The deployment settings layout already has padding.
				css={{ paddingTop: 0 }}
				actions={
					<Button
						endIcon={<OpenInNewIcon />}
						href={docs("/admin/provisioners")}
					>
						Create a provisioner
					</Button>
				}
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
