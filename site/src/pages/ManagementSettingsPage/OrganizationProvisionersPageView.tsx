import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import type { BuildInfoResponse, ProvisionerDaemon } from "api/typesGenerated";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { ProvisionerGroup } from "modules/provisioners/ProvisionerGroup";
import type { FC } from "react";
import { docs } from "utils/docs";

export interface ProvisionersByGroup {
	builtin: ProvisionerDaemon[];
	psk: ProvisionerDaemon[];
	userAuth: ProvisionerDaemon[];
	keys: Map<string, ProvisionerDaemon[]>;
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
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader
					title="Provisioners"
					badges={<FeatureStageBadge contentType="beta" size="lg" />}
				/>
				<Button
					endIcon={<OpenInNewIcon />}
					target="_blank"
					href={docs("/admin/provisioners")}
				>
					Create a provisioner
				</Button>
			</Stack>

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
