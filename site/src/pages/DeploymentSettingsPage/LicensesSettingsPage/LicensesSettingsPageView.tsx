import type { Interpolation, Theme } from "@emotion/react";
import MuiLink from "@mui/material/Link";
import Skeleton from "@mui/material/Skeleton";
import type { GetLicensesResponse } from "api/api";
import type { Feature, UserStatusChangeCount } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { PlusIcon, RotateCwIcon, SparklesIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { AIGovernanceUsersConsumption } from "./AIGovernanceUsersConsumptionChart";
import { LicenseCard } from "./LicenseCard";
import { LicenseSeatConsumptionChart } from "./LicenseSeatConsumptionChart";
import { ManagedAgentsConsumption } from "./ManagedAgentsConsumption";

type Props = {
	showSuccessDialog: boolean;
	onDismissSuccessDialog: () => void;
	isLoading: boolean;
	userLimitActual?: number;
	userLimitLimit?: number;
	licenses?: GetLicensesResponse[];
	isRemovingLicense: boolean;
	isRefreshing: boolean;
	removeLicense: (licenseId: number) => void;
	refreshEntitlements: () => void;
	activeUsers: UserStatusChangeCount[] | undefined;
	managedAgentFeature?: Feature;
	aiGovernanceUserFeature?: Feature;
};

const LicensesSettingsPageView: FC<Props> = ({
	showSuccessDialog,
	onDismissSuccessDialog,
	isLoading,
	userLimitActual,
	userLimitLimit,
	licenses,
	isRemovingLicense,
	isRefreshing,
	removeLicense,
	refreshEntitlements,
	activeUsers,
	managedAgentFeature,
	aiGovernanceUserFeature,
}) => {
	return (
		<>
			<LicenseUpgradeSuccessDialog
				open={showSuccessDialog}
				onClose={onDismissSuccessDialog}
			/>

			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader>
					<SettingsHeaderTitle>Licenses</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Manage licenses to unlock Premium features.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Stack direction="row" spacing={2}>
					<Button variant="outline" asChild>
						<Link to="/deployment/licenses/add">
							<PlusIcon />
							Add a license
						</Link>
					</Button>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								disabled={isRefreshing}
								onClick={refreshEntitlements}
								variant="outline"
							>
								<Spinner loading={isRefreshing}>
									<RotateCwIcon />
								</Spinner>
								Refresh
							</Button>
						</TooltipTrigger>
						<TooltipContent side="bottom" className="max-w-xs">
							Refresh license entitlements. This is done automatically every 10
							minutes.
						</TooltipContent>
					</Tooltip>
				</Stack>
			</Stack>

			<div className="flex flex-col gap-4">
				{isLoading && (
					<Skeleton className="rounded" variant="rectangular" height={78} />
				)}

				{!isLoading && licenses && licenses?.length > 0 && (
					<Stack spacing={4} className="licenses">
						{[...(licenses ?? [])]
							?.sort(
								(a, b) =>
									new Date(b.claims.license_expires).valueOf() -
									new Date(a.claims.license_expires).valueOf(),
							)
							.map((license) => (
								<LicenseCard
									key={license.id}
									license={license}
									userLimitActual={userLimitActual}
									userLimitLimit={userLimitLimit}
									isRemoving={isRemovingLicense}
									onRemove={removeLicense}
								/>
							))}
					</Stack>
				)}

				{!isLoading && licenses === null && (
					<div css={styles.root}>
						<Stack alignItems="center" spacing={1}>
							<Stack alignItems="center" spacing={0.5}>
								<span css={styles.title}>
									You don&apos;t have any licenses!
								</span>
								<span css={styles.description}>
									You&apos;re missing out on high availability, RBAC, quotas,
									and much more. Contact{" "}
									<MuiLink href="mailto:sales@coder.com">sales</MuiLink> or{" "}
									<MuiLink href="https://coder.com/trial">
										request a trial license
									</MuiLink>{" "}
									to get started.
								</span>
							</Stack>
						</Stack>
					</div>
				)}

				{licenses && licenses.length > 0 && (
					<LicenseSeatConsumptionChart
						limit={userLimitLimit}
						data={activeUsers?.map((i) => ({
							date: i.date,
							users: i.count,
							limit: 80,
						}))}
					/>
				)}

				{licenses && licenses.length > 0 && (
					<>
						<ManagedAgentsConsumption
							managedAgentFeature={managedAgentFeature}
						/>
						<AIGovernanceUsersConsumption
							aiGovernanceUserFeature={aiGovernanceUserFeature}
						/>
					</>
				)}
			</div>
		</>
	);
};

interface LicenseUpgradeSuccessDialogProps {
	open: boolean;
	onClose: () => void;
}

const LicenseUpgradeSuccessDialog: FC<LicenseUpgradeSuccessDialogProps> = ({
	open,
	onClose,
}) => {
	return (
		<Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
			<DialogContent className="max-w-md">
				<div className="flex flex-col items-center text-center gap-4">
					{/* Placeholder for Blink animation - replace with Rive/Lottie animation
					   following the 6th brand ident from https://coder.com/brand#4-idents */}
					<div className="flex items-center justify-center size-20 rounded-full bg-surface-secondary">
						<SparklesIcon className="size-10 text-content-link" />
					</div>
				</div>

				<DialogHeader className="items-center">
					<DialogTitle className="text-center">
						License added successfully
					</DialogTitle>
					<DialogDescription className="text-center">
						Your Premium features are now unlocked. Enjoy high availability,
						RBAC, quotas, and much more.
					</DialogDescription>
				</DialogHeader>

				<DialogFooter className="sm:justify-center">
					<Button onClick={onClose}>Get started</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

const styles = {
	title: {
		fontSize: 16,
	},

	root: (theme) => ({
		minHeight: 240,
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		borderRadius: 8,
		border: `1px solid ${theme.palette.divider}`,
		padding: 48,
	}),

	description: (theme) => ({
		color: theme.palette.text.secondary,
		textAlign: "center",
		maxWidth: 464,
		marginTop: 8,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default LicensesSettingsPageView;
