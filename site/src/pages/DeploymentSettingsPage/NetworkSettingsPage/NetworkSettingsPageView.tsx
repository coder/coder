import type { SerpentOption } from "api/typesGenerated";
import { Badges, DisabledBadge, EnabledBadge } from "components/Badges/Badges";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
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
				actions={<SettingsHeaderDocsLink href={docs("/admin/networking")} />}
			>
				<SettingsHeaderTitle>Network</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure your deployment connectivity.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<OptionsTable
				options={options.filter((o) =>
					deploymentGroupHasParent(o.group, "Networking"),
				)}
			/>
		</div>

		<div>
			<SettingsHeader
				actions={
					<SettingsHeaderDocsLink
						href={docs("/admin/networking/port-forwarding")}
					/>
				}
			>
				<SettingsHeaderTitle level="h2" hierarchy="secondary">
					Port Forwarding
				</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Port forwarding lets developers securely access processes on their
					Coder workspace from a local machine.
				</SettingsHeaderDescription>
			</SettingsHeader>

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
