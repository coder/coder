import type { FC } from "react";
import {
	type ProvisionerKeyDaemons,
	ProvisionerKeyIDBuiltIn,
	ProvisionerKeyIDPSK,
	ProvisionerKeyIDUserAuth,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { PremiumPaywall } from "#/modules/paywall/PremiumPaywall";
import type { Permissions } from "#/modules/permissions";
import { docs } from "#/utils/docs";
import { ProvisionerKeyRow } from "./ProvisionerKeyRow";

// If the user using provisioner keys for external provisioners you're unlikely to
// want to keep the built-in provisioners.
const HIDDEN_PROVISIONER_KEYS = [
	ProvisionerKeyIDBuiltIn,
	ProvisionerKeyIDUserAuth,
	ProvisionerKeyIDPSK,
];

interface OrganizationProvisionerKeysPageViewProps {
	showPaywall: boolean | undefined;
	provisionerKeyDaemons: ProvisionerKeyDaemons[] | undefined;
	error: unknown;
	permissions: Permissions;
	onRetry: () => void;
}

export const OrganizationProvisionerKeysPageView: FC<
	OrganizationProvisionerKeysPageViewProps
> = ({ showPaywall, provisionerKeyDaemons, error, permissions, onRetry }) => {
	const filteredProvisionerKeyDaemons = provisionerKeyDaemons?.filter(
		(pkd) => !HIDDEN_PROVISIONER_KEYS.includes(pkd.key.id),
	);

	return (
		<section className="w-full max-w-screen-2xl pb-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Provisioner Keys</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Manage provisioner keys used to authenticate provisioner instances.{" "}
					<SettingsHeaderDocsLink href={docs("/admin/provisioners")} />
				</SettingsHeaderDescription>
			</SettingsHeader>

			{showPaywall ? (
				<PremiumPaywall
					source="provisioner_keys"
					message="Provisioners"
					description="Scoped authentication keys for org provisioners."
					features={[
						"Scoped per organization & tag",
						"Recommended provisioner authentication",
						"Rotate keys without downtime",
						"Fully isolated per organization",
					]}
					canViewPremium={permissions.viewAllLicenses}
				/>
			) : (
				<Table className="mt-6">
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Tags</TableHead>
							<TableHead>Active Provisioners</TableHead>
							<TableHead>Created</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{filteredProvisionerKeyDaemons ? (
							filteredProvisionerKeyDaemons.length === 0 ? (
								<TableEmpty
									message="No provisioner keys"
									description="Create your first provisioner key to authenticate external provisioner daemons."
								/>
							) : (
								filteredProvisionerKeyDaemons.map((pkd) => (
									<ProvisionerKeyRow
										key={pkd.key.id}
										provisionerKey={pkd.key}
										provisioners={pkd.daemons}
										defaultIsOpen={false}
									/>
								))
							)
						) : error ? (
							<TableEmpty
								message="Error loading provisioner keys"
								cta={
									<Button onClick={onRetry} size="sm">
										Retry
									</Button>
								}
							/>
						) : (
							<TableLoader />
						)}
					</TableBody>
				</Table>
			)}
		</section>
	);
};
