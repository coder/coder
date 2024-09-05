import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { Provisioner } from "modules/provisioners/Provisioner";
import type { FC } from "react";
import { docs } from "utils/docs";
import type { ProvisionersByGroup } from "./OrganizationProvisionersPage";
import { ProvisionerGroup } from "modules/provisioners/ProvisionerGroup";

interface OrganizationProvisionersPageViewProps {
	provisioners: ProvisionersByGroup;
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
						target="_blank"
						href={docs("/admin/provisioners")}
					>
						Create a provisioner
					</Button>
				}
			>
				<PageHeaderTitle>Provisioners</PageHeaderTitle>
			</PageHeader>
			<Stack spacing={4.5}>
				{provisioners.builtin.length > 0 && (
					<ProvisionerGroup
						type="builtin"
						provisioners={provisioners.builtin}
					/>
				)}
				{provisioners.psk.length > 0 && (
					<ProvisionerGroup type="psk" provisioners={provisioners.psk} />
				)}
				{[...provisioners.keys].map(([keyId, provisioners]) => (
					<ProvisionerGroup
						key={keyId}
						keyName={keyId}
						type="key"
						provisioners={provisioners}
					/>
				))}
			</Stack>
		</div>
	);
};
