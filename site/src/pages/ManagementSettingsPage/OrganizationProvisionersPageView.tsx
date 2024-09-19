import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import type { BuildInfoResponse, ProvisionerDaemon } from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { ProvisionerGroup } from "modules/provisioners/ProvisionerGroup";
import type { FC } from "react";
import { docs } from "utils/docs";

export interface ProvisionersByGroup {
	builtin: ProvisionerDaemon[];
	psk: ProvisionerDaemon[];
	keys: Map<string, ProvisionerDaemon[]>;
}

interface OrganizationProvisionersPageViewProps {
	buildInfo?: BuildInfoResponse;
	provisioners: ProvisionersByGroup;
}

export const OrganizationProvisionersPageView: FC<
	OrganizationProvisionersPageViewProps
> = ({ buildInfo, provisioners }) => {
	const isEmpty =
		provisioners.builtin.length +
			provisioners.psk.length +
			provisioners.keys.size ===
		0;

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
				{isEmpty && (
					<EmptyState
						message="No provisioners"
						description="A provisioner is required before you can create templates and workspaces. You can connect your first provisioner by following our documentation."
						cta={
							<Button
								endIcon={<OpenInNewIcon />}
								target="_blank"
								href={docs("/admin/provisioners")}
							>
								Show me how to create a provisioner
							</Button>
						}
					/>
				)}
				{provisioners.builtin.length > 0 && (
					<ProvisionerGroup
						buildInfo={buildInfo}
						type="builtin"
						provisioners={provisioners.builtin}
					/>
				)}
				{provisioners.psk.length > 0 && (
					<ProvisionerGroup
						buildInfo={buildInfo}
						type="psk"
						provisioners={provisioners.psk}
					/>
				)}
				{[...provisioners.keys].map(([keyId, provisioners]) => (
					<ProvisionerGroup
						key={keyId}
						buildInfo={buildInfo}
						keyName={keyId}
						type="key"
						provisioners={provisioners}
					/>
				))}
			</Stack>
		</div>
	);
};
