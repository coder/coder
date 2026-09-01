import type { FC } from "react";
import type { SerpentOption } from "#/api/typesGenerated";
import {
	BadgeGroup,
	DisabledBadge,
	EnabledBadge,
} from "#/components/Badge/PresetBadges";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	deploymentGroupHasParent,
	useDeploymentOptions,
} from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import OptionsTable from "../OptionsTable";

type NetworkSettingsPageViewProps = {
	options: SerpentOption[];
};

export const NetworkSettingsPageView: FC<NetworkSettingsPageViewProps> = ({
	options,
}) => (
	<div className="flex flex-col gap-12">
		<div>
			<SettingsHeader>
				<SettingsHeaderTitle>Network</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure your deployment connectivity.{" "}
					<SettingsHeaderDocsLink
						href={docs("/admin/networking")}
						context="about deployment networking"
					/>
				</SettingsHeaderDescription>
			</SettingsHeader>

			<OptionsTable
				options={options.filter((o) =>
					deploymentGroupHasParent(o.group, "Networking"),
				)}
			/>
		</div>

		<div>
			<SettingsHeader>
				<SettingsHeaderTitle level="h2" hierarchy="secondary">
					Port Forwarding
				</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Port forwarding lets developers securely access processes on their
					Coder workspace from a local machine.{" "}
					<SettingsHeaderDocsLink
						href={docs("/admin/networking/port-forwarding")}
						context="about port forwarding"
					/>
				</SettingsHeaderDescription>
			</SettingsHeader>

			<BadgeGroup>
				{useDeploymentOptions(options, "Wildcard Access URL")[0].value !==
				"" ? (
					<EnabledBadge />
				) : (
					<DisabledBadge />
				)}
			</BadgeGroup>
		</div>
	</div>
);
