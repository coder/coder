import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import type {
	BuildInfoResponse,
	HealthMessage,
	ProvisionerDaemon,
} from "api/typesGenerated";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { ProvisionerGroup } from "modules/provisioners/ProvisionerGroup";
import type { FC } from "react";
import { docs } from "utils/docs";

export interface ProvisionerDaemonWithWarnings extends ProvisionerDaemon {
	readonly warnings?: readonly HealthMessage[];
}

export interface ProvisionersByGroup {
	builtin: ProvisionerDaemonWithWarnings[];
	psk: ProvisionerDaemonWithWarnings[];
	keys: Map<string, ProvisionerDaemonWithWarnings[]>;
}

interface OrganizationProvisionersPageViewProps {
	buildInfo?: BuildInfoResponse;
	provisioners: ProvisionersByGroup;
}

export const OrganizationProvisionersPageView: FC<
	OrganizationProvisionersPageViewProps
> = ({ buildInfo, provisioners }) => {
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
