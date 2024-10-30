import type { SerpentOption } from "api/typesGenerated";
import {
	Badges,
	DisabledBadge,
	EnabledBadge,
	PremiumBadge,
} from "components/Badges/Badges";
// import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
// import { Stack } from "components/Stack/Stack";
import { Button } from "@/components/ui/button";
import type { FC } from "react";

export type PremiumPageViewProps = { isPremium: boolean };

export const PremiumPageView: FC<PremiumPageViewProps> = ({ isPremium }) => {
	return (
		<div className="pr-32">
			<div className="flex flex-row justify-between align-baseline">
				<div>
					<h1>Premium</h1>
					<p className="max-w-lg">
						Overview of all features that are not covered by the OSS license.
					</p>
				</div>
				<Button variant="default">Upgrade to Premium</Button>
			</div>

			<h2>Organizations</h2>
			<p className="max-w-lg">
				Create multiple organizations within a single Coder deployment, allowing
				several platform teams to operate with isolated users, templates, and
				distinct underlying infrastructure.
			</p>

			<h2>Appearance</h2>
			<p>Customize the look and feel of your Coder deployment.</p>

			<h2>Observability</h2>
			<p>Allow auditors to monitor user operations in your deployment.</p>

			<h2>Provisioners</h2>
			<p>Provisioners run your Terraform to create templates and workspaces.</p>

			<h2>Custom Roles</h2>
			<p>
				Create custom roles to grant users a tailored set of granular
				permissions.
			</p>

			<h2>IdP Sync</h2>
			<p>
				Configure group and role mappings to manage permissions outside of
				Coder.{" "}
			</p>
		</div>
	);
};
