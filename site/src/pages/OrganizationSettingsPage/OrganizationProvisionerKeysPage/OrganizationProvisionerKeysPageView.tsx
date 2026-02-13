import {
	type ProvisionerKeyDaemons,
	ProvisionerKeyIDBuiltIn,
	ProvisionerKeyIDPSK,
	ProvisionerKeyIDUserAuth,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import { PaywallPremium } from "components/Paywall/PaywallPremium";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import type { FC } from "react";
import { docs } from "utils/docs";
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
	onRetry: () => void;
}

export const OrganizationProvisionerKeysPageView: FC<
	OrganizationProvisionerKeysPageViewProps
> = ({ showPaywall, provisionerKeyDaemons, error, onRetry }) => {
	return (
		<section className="w-full max-w-screen-2xl pb-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Provisioner Keys</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Manage provisioner keys used to authenticate provisioner instances.{" "}
					<Link href={docs("/admin/provisioners")}>View docs</Link>
				</SettingsHeaderDescription>
			</SettingsHeader>

			{showPaywall ? (
				<PaywallPremium
					message="Provisioners"
					description="Provisioners run your Terraform to create templates and workspaces. You need a Premium license to use this feature for multiple organizations."
					documentationLink={docs("/")}
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
						{provisionerKeyDaemons ? (
							provisionerKeyDaemons.length === 0 ? (
								<TableRow>
									<TableCell colSpan={5}>
										<EmptyState
											message="No provisioner keys"
											description="Create your first provisioner key to authenticate external provisioner daemons."
										/>
									</TableCell>
								</TableRow>
							) : (
								provisionerKeyDaemons
									.filter(
										(pkd) => !HIDDEN_PROVISIONER_KEYS.includes(pkd.key.id),
									)
									.map((pkd) => (
										<ProvisionerKeyRow
											key={pkd.key.id}
											provisionerKey={pkd.key}
											provisioners={pkd.daemons}
											defaultIsOpen={false}
										/>
									))
							)
						) : error ? (
							<TableRow>
								<TableCell colSpan={5}>
									<EmptyState
										message="Error loading provisioner keys"
										cta={
											<Button onClick={onRetry} size="sm">
												Retry
											</Button>
										}
									/>
								</TableCell>
							</TableRow>
						) : (
							<TableRow>
								<TableCell colSpan={999}>
									<Loader />
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</Table>
			)}
		</section>
	);
};
