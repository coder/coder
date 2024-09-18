import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import type { ProvisionerDaemon } from "api/typesGenerated";
import { FeatureBadge } from "components/FeatureBadge/FeatureBadge";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
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
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader
					title="Provisioners"
					tooltip={<FeatureBadge type="beta" variant="interactive" size="lg" />}
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
				{provisioners.map((provisioner) => (
					<Provisioner key={provisioner.id} provisioner={provisioner} />
				))}
			</Stack>
		</div>
	);
};
