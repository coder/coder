import type { CSSObject, Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Paper from "@mui/material/Paper";
import type { GetLicensesResponse } from "api/api";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { compareAsc } from "date-fns";
import dayjs from "dayjs";
import { useTimeSync } from "hooks/useTimeSync";
import { type FC, useState } from "react";

type LicenseCardProps = {
	license: GetLicensesResponse;
	userLimitActual?: number;
	userLimitLimit?: number;
	onRemove: (licenseId: number) => void;
	isRemoving: boolean;
};

export const LicenseCard: FC<LicenseCardProps> = ({
	license,
	userLimitActual,
	userLimitLimit,
	onRemove,
	isRemoving,
}) => {
	const time = useTimeSync({ idealRefreshIntervalMs: 30_000 });
	const [licenseIDMarkedForRemoval, setLicenseIDMarkedForRemoval] = useState<
		number | undefined
	>(undefined);

	const currentUserLimit = license.claims.features.user_limit || userLimitLimit;

	const licenseType = license.claims.trial
		? "Trial"
		: license.claims.feature_set?.toLowerCase() === "premium"
			? "Premium"
			: "Enterprise";

	return (
		<Paper
			key={license.id}
			elevation={2}
			css={styles.licenseCard}
			className="license-card"
		>
			<ConfirmDialog
				type="delete"
				hideCancel={false}
				open={licenseIDMarkedForRemoval !== undefined}
				onConfirm={() => {
					if (!licenseIDMarkedForRemoval) {
						return;
					}
					onRemove(licenseIDMarkedForRemoval);
					setLicenseIDMarkedForRemoval(undefined);
				}}
				onClose={() => setLicenseIDMarkedForRemoval(undefined)}
				title="Confirm License Removal"
				confirmLoading={isRemoving}
				confirmText="Remove"
				description="Removing this license will disable all Premium features. You add a new license at any time."
			/>
			<Stack
				direction="row"
				spacing={2}
				css={styles.cardContent}
				justifyContent="left"
				alignItems="center"
			>
				<span css={styles.licenseId}>#{license.id}</span>
				<span css={styles.accountType} className="account-type">
					{licenseType}
				</span>
				<Stack
					direction="row"
					justifyContent="right"
					spacing={8}
					alignItems="self-end"
					style={{
						flex: 1,
					}}
				>
					<Stack direction="column" spacing={0} alignItems="center">
						<span css={styles.secondaryMaincolor}>Users</span>
						<span css={styles.userLimit} className="user-limit">
							{userLimitActual} {` / ${currentUserLimit || "Unlimited"}`}
						</span>
					</Stack>
					<Stack
						direction="column"
						spacing={0}
						alignItems="center"
						width="134px" // standardize width of date column
					>
						{compareAsc(new Date(license.claims.license_expires * 1000), time) <
						1 ? (
							<Pill css={styles.expiredBadge} type="error">
								Expired
							</Pill>
						) : (
							<span css={styles.secondaryMaincolor}>Valid Until</span>
						)}
						<span css={styles.licenseExpires} className="license-expires">
							{dayjs
								.unix(license.claims.license_expires)
								.format("MMMM D, YYYY")}
						</span>
					</Stack>
					<Stack spacing={2}>
						<Button
							css={styles.removeButton}
							variant="contained"
							size="small"
							onClick={() => setLicenseIDMarkedForRemoval(license.id)}
							className="remove-button"
						>
							Remove&hellip;
						</Button>
					</Stack>
				</Stack>
			</Stack>
		</Paper>
	);
};

const styles = {
	userLimit: (theme) => ({
		color: theme.palette.text.primary,
	}),
	licenseCard: (theme) => ({
		...(theme.typography.body2 as CSSObject),
		padding: 16,
	}),
	cardContent: {},
	licenseId: (theme) => ({
		color: theme.palette.secondary.main,
		fontSize: 18,
		fontWeight: 600,
	}),
	accountType: {
		fontWeight: 600,
		fontSize: 18,
		alignItems: "center",
		textTransform: "capitalize",
	},
	licenseExpires: (theme) => ({
		color: theme.palette.text.secondary,
	}),
	expiredBadge: {
		marginBottom: 4,
	},
	secondaryMaincolor: (theme) => ({
		color: theme.palette.text.secondary,
	}),
	removeButton: (theme) => ({
		color: theme.palette.error.main,
		"&:hover": {
			backgroundColor: "transparent",
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
