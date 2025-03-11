import type { SerpentOption } from "api/typesGenerated";
import { Badges, DisabledBadge, EnabledBadge } from "components/Badges/Badges";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import {
	deploymentGroupHasParent,
	useDeploymentOptions,
} from "utils/deployOptions";
import { docs } from "utils/docs";
import OptionsTable from "../OptionsTable";

export type NetworkSettingsPageViewProps = {
	options: SerpentOption[];
};

export const NetworkSettingsPageView: FC<NetworkSettingsPageViewProps> = ({
	options,
}) => (
	<Stack direction="column" spacing={6}>
		<div>
			<SettingsHeader
				title="Network"
				description="Configure your deployment connectivity."
				docsHref={docs("/admin/networking")}
			/>
			<OptionsTable
				options={options.filter((o) =>
					deploymentGroupHasParent(o.group, "Networking"),
				)}
			/>
		</div>

		<div>
			<SettingsHeader
				title="Port Forwarding"
				hierarchy="secondary"
				description="Port forwarding lets developers securely access processes on their Coder workspace from a local machine."
				docsHref={docs("/admin/networking/port-forwarding")}
			/>

			<Badges>
				{useDeploymentOptions(options, "Wildcard Access URL")[0].value !==
				"" ? (
					<EnabledBadge />
				) : (
					<DisabledBadge />
				)}
			</Badges>
		</div>
	</Stack>
);
