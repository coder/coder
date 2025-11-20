import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import MuiLink from "@mui/material/Link";
import Skeleton from "@mui/material/Skeleton";
import type { GetLicensesResponse } from "api/api";
import type { Feature, UserStatusChangeCount } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import MiniTooltip from "components/MiniTooltip/MiniTooltip";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { useWindowSize } from "hooks/useWindowSize";
import { PlusIcon, RotateCwIcon } from "lucide-react";
import type { FC } from "react";
import Confetti from "react-confetti";
import { Link } from "react-router";
import { LicenseCard } from "./LicenseCard";
import { LicenseSeatConsumptionChart } from "./LicenseSeatConsumptionChart";
import { ManagedAgentsConsumption } from "./ManagedAgentsConsumption";

type Props = {
	showConfetti: boolean;
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
};

const LicensesSettingsPageView: FC<Props> = ({
	showConfetti,
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
}) => {
	const theme = useTheme();
	const { width, height } = useWindowSize();

	return (
		<>
			<Confetti
				// For some reason this overflows the window and adds scrollbars if we don't subtract here.
				width={width - 1}
				height={height - 1}
				numberOfPieces={showConfetti ? 200 : 0}
				colors={[theme.palette.primary.main, theme.palette.secondary.main]}
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
					<MiniTooltip title="Refresh license entitlements. This is done automatically every 10 minutes.">
						<Button
							disabled={isRefreshing}
							onClick={refreshEntitlements}
							variant="outline"
						>
							<Spinner loading={isRefreshing}>
								<RotateCwIcon className="size-icon-xs" />
							</Spinner>
							Refresh
						</Button>
					</MiniTooltip>
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
					<ManagedAgentsConsumption managedAgentFeature={managedAgentFeature} />
				)}
			</div>
		</>
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
