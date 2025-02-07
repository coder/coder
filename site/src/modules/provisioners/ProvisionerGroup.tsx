import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import BusinessIcon from "@mui/icons-material/Business";
import PersonIcon from "@mui/icons-material/Person";
import TagIcon from "@mui/icons-material/Sell";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import type { BuildInfoResponse, ProvisionerDaemon } from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { StatusIndicatorDot } from "components/StatusIndicator/StatusIndicator";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import { type FC, useState } from "react";
import { createDayString } from "utils/createDayString";
import { docs } from "utils/docs";
import { ProvisionerTag } from "./ProvisionerTag";

type ProvisionerGroupType = "builtin" | "userAuth" | "psk" | "key";

interface ProvisionerGroupProps {
	readonly buildInfo: BuildInfoResponse;
	readonly keyName: string;
	readonly keyTags: Record<string, string>;
	readonly type: ProvisionerGroupType;
	readonly provisioners: readonly ProvisionerDaemon[];
}

function isSimpleTagSet(tags: Record<string, string>) {
	const numberOfExtraTags = Object.keys(tags).filter(
		(key) => key !== "scope" && key !== "owner",
	).length;
	return (
		numberOfExtraTags === 0 && tags.scope === "organization" && !tags.owner
	);
}

export const ProvisionerGroup: FC<ProvisionerGroupProps> = ({
	buildInfo,
	keyName,
	keyTags,
	type,
	provisioners,
}) => {
	const theme = useTheme();

	const [showDetails, setShowDetails] = useState(false);

	const firstProvisioner = provisioners[0];
	if (!firstProvisioner) {
		return null;
	}

	const daemonScope = firstProvisioner.tags.scope || "organization";
	const allProvisionersAreSameVersion = provisioners.every(
		(it) => it.version === firstProvisioner.version,
	);
	const provisionerVersion = allProvisionersAreSameVersion
		? firstProvisioner.version
		: null;
	const provisionerCount =
		provisioners.length === 1
			? "1 provisioner"
			: `${provisioners.length} provisioners`;
	const extraTags = Object.entries(keyTags).filter(
		([key]) => key !== "scope" && key !== "owner",
	);

	let warnings = 0;
	let provisionersWithWarnings = 0;
	const provisionersWithWarningInfo = provisioners.map((it) => {
		const outOfDate = it.version !== buildInfo.version;
		const warningCount = outOfDate ? 1 : 0;
		warnings += warningCount;
		if (warnings > 0) {
			provisionersWithWarnings++;
		}

		return { ...it, warningCount, outOfDate };
	});

	const hasWarning = warnings > 0;
	const warningsCount =
		warnings === 0
			? "No warnings"
			: warnings === 1
				? "1 warning"
				: `${warnings} warnings`;
	const provisionersWithWarningsCount =
		provisionersWithWarnings === 1
			? "1 provisioner"
			: `${provisionersWithWarnings} provisioners`;

	const hasMultipleTagVariants =
		(type === "psk" || type === "userAuth") &&
		provisioners.some((it) => !isSimpleTagSet(it.tags));

	return (
		<div
			css={[
				{
					borderRadius: 8,
					border: `1px solid ${theme.palette.divider}`,
					fontSize: 14,
				},
				hasWarning && styles.warningBorder,
			]}
		>
			<header
				css={{
					padding: 24,
					display: "flex",
					alignItems: "center",
					justifyContent: "space-between",
					gap: 24,
				}}
			>
				<div css={{ display: "flex", alignItems: "center", gap: 16 }}>
					<StatusIndicatorDot variant={hasWarning ? "warning" : "success"} />
					<div
						css={{
							display: "flex",
							flexDirection: "column",
							lineHeight: 1.5,
						}}
					>
						{type === "builtin" && (
							<>
								<BuiltinProvisionerTitle />
								<span css={{ color: theme.palette.text.secondary }}>
									{provisionerCount} &mdash; Built-in
								</span>
							</>
						)}

						{type === "userAuth" && <UserAuthProvisionerTitle />}

						{type === "psk" && <PskProvisionerTitle />}
						{type === "key" && (
							<h4 css={styles.groupTitle}>Key group &ndash; {keyName}</h4>
						)}
						{type !== "builtin" && (
							<span css={{ color: theme.palette.text.secondary }}>
								{provisionerCount} &mdash;{" "}
								{provisionerVersion ? (
									<code>{provisionerVersion}</code>
								) : (
									<span>Multiple versions</span>
								)}
							</span>
						)}
					</div>
				</div>
				<div
					css={{
						marginLeft: "auto",
						display: "flex",
						flexWrap: "wrap",
						gap: 12,
						justifyContent: "right",
					}}
				>
					{!hasMultipleTagVariants ? (
						<Tooltip title="Scope">
							<Pill
								size="lg"
								icon={
									daemonScope === "organization" ? (
										<BusinessIcon />
									) : (
										<PersonIcon />
									)
								}
							>
								<span css={{ textTransform: "capitalize" }}>{daemonScope}</span>
							</Pill>
						</Tooltip>
					) : (
						<Pill size="lg" icon={<TagIcon />}>
							Multiple tags
						</Pill>
					)}
					{type === "key" &&
						extraTags.map(([key, value]) => (
							<ProvisionerTag key={key} tagName={key} tagValue={value} />
						))}
				</div>
			</header>

			{showDetails && (
				<div
					css={{
						padding: "0 24px 24px",
						display: "grid",
						gap: 12,
						gridTemplateColumns: "repeat(auto-fill, minmax(385px, 1fr))",
					}}
				>
					{provisionersWithWarningInfo.map((provisioner) => (
						<div
							key={provisioner.id}
							css={[
								{
									borderRadius: 8,
									border: `1px solid ${theme.palette.divider}`,
									fontSize: 14,
									padding: "14px 18px",
								},
								provisioner.warningCount > 0 && styles.warningBorder,
							]}
						>
							<Stack
								direction="row"
								justifyContent="space-between"
								alignItems="center"
							>
								<div css={{ lineHeight: 1.5 }}>
									<h4 css={{ fontWeight: 500, margin: 0 }}>
										{provisioner.name}
									</h4>
									<span
										css={{ color: theme.palette.text.secondary, fontSize: 12 }}
									>
										{type === "builtin" ? (
											<span>Built-in</span>
										) : (
											<>
												<ProvisionerVersionPopover
													buildInfo={buildInfo}
													provisioner={provisioner}
												/>{" "}
												&mdash;{" "}
												{provisioner.last_seen_at && (
													<span data-chromatic="ignore">
														Last seen{" "}
														{createDayString(provisioner.last_seen_at)}
													</span>
												)}
											</>
										)}
									</span>
								</div>
								{hasMultipleTagVariants && (
									<InlineProvisionerTags tags={provisioner.tags} />
								)}
							</Stack>
						</div>
					))}
				</div>
			)}

			<div
				css={{
					borderTop: `1px solid ${theme.palette.divider}`,
					display: "flex",
					alignItems: "center",
					justifyContent: "space-between",
					padding: "8px 8px 8px 24px",
					fontSize: 12,
					color: theme.palette.text.secondary,
				}}
			>
				<span>
					{warningsCount} from{" "}
					{hasWarning ? provisionersWithWarningsCount : provisionerCount}
				</span>
				<Button
					variant="text"
					css={{
						display: "flex",
						alignItems: "center",
						gap: 4,
						color: theme.roles.info.text,
						fontSize: "inherit",
					}}
					onClick={() => setShowDetails((it) => !it)}
				>
					{showDetails ? "Hide" : "Show"} provisioner details{" "}
					<DropdownArrow close={showDetails} />
				</Button>
			</div>
		</div>
	);
};

interface ProvisionerVersionPopoverProps {
	buildInfo: BuildInfoResponse;
	provisioner: ProvisionerDaemon;
}

const ProvisionerVersionPopover: FC<ProvisionerVersionPopoverProps> = ({
	buildInfo,
	provisioner,
}) => {
	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<span>
					{provisioner.version === buildInfo.version
						? "Up to date"
						: "Out of date"}
				</span>
			</PopoverTrigger>
			<PopoverContent
				transformOrigin={{ vertical: -8, horizontal: 0 }}
				css={{
					"& .MuiPaper-root": {
						padding: "20px 20px 8px",
						maxWidth: 340,
					},
				}}
			>
				<h4 css={styles.versionPopoverTitle}>Release version</h4>
				<p css={styles.text}>{provisioner.version}</p>
				<h4 css={styles.versionPopoverTitle}>Protocol version</h4>
				<p css={styles.text}>{provisioner.api_version}</p>
				{provisioner.api_version !== buildInfo.provisioner_api_version && (
					<p css={[styles.text, { fontSize: 13 }]}>
						This provisioner is out of date. You may experience issues when
						using a provisioner version that doesn’t match your Coder
						deployment. Please upgrade to a newer version.{" "}
						<Link href={docs("/")}>Learn more…</Link>
					</p>
				)}
			</PopoverContent>
		</Popover>
	);
};

interface InlineProvisionerTagsProps {
	tags: Record<string, string>;
}

const InlineProvisionerTags: FC<InlineProvisionerTagsProps> = ({ tags }) => {
	const daemonScope = tags.scope || "organization";
	const iconScope =
		daemonScope === "organization" ? <BusinessIcon /> : <PersonIcon />;

	const extraTags = Object.entries(tags).filter(
		([tag]) => tag !== "scope" && tag !== "owner",
	);

	if (extraTags.length === 0) {
		return (
			<Pill icon={iconScope}>
				<span css={{ textTransform: "capitalize" }}>{daemonScope}</span>
			</Pill>
		);
	}

	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<Pill icon={iconScope}>
					{extraTags.length === 1 ? "+ 1 tag" : `+ ${extraTags.length} tags`}
				</Pill>
			</PopoverTrigger>
			<PopoverContent
				transformOrigin={{ vertical: -8, horizontal: 0 }}
				css={{
					"& .MuiPaper-root": {
						padding: 20,
						minWidth: "unset",
						maxWidth: 340,
						width: "fit-content",
					},
				}}
			>
				<div
					css={{
						marginLeft: "auto",
						display: "flex",
						flexWrap: "wrap",
						gap: 12,
						justifyContent: "right",
					}}
				>
					{extraTags.map(([key, value]) => (
						<ProvisionerTag key={key} tagName={key} tagValue={value} />
					))}
				</div>
			</PopoverContent>
		</Popover>
	);
};

const BuiltinProvisionerTitle: FC = () => {
	return (
		<h4 css={styles.groupTitle}>
			<Stack direction="row" alignItems="end" spacing={1}>
				<span>Built-in provisioners</span>
				<HelpTooltip>
					<HelpTooltipTrigger />
					<HelpTooltipContent>
						<HelpTooltipTitle>Built-in provisioners</HelpTooltipTitle>
						<HelpTooltipText>
							These provisioners are running as part of a coderd instance.
							Built-in provisioners are only available for the default
							organization. <Link href={docs("/")}>Learn more&hellip;</Link>
						</HelpTooltipText>
					</HelpTooltipContent>
				</HelpTooltip>
			</Stack>
		</h4>
	);
};

const UserAuthProvisionerTitle: FC = () => {
	return (
		<h4 css={styles.groupTitle}>
			<Stack direction="row" alignItems="end" spacing={1}>
				<span>User-authenticated provisioners</span>
				<HelpTooltip>
					<HelpTooltipTrigger />
					<HelpTooltipContent>
						<HelpTooltipTitle>User-authenticated provisioners</HelpTooltipTitle>
						<HelpTooltipText>
							These provisioners are connected by users using the{" "}
							<code>coder</code> CLI, and are authorized by the users
							credentials. They can be tagged to only run provisioner jobs for
							that user. User-authenticated provisioners are only available for
							the default organization.{" "}
							<Link href={docs("/")}>Learn more&hellip;</Link>
						</HelpTooltipText>
					</HelpTooltipContent>
				</HelpTooltip>
			</Stack>
		</h4>
	);
};

const PskProvisionerTitle: FC = () => {
	return (
		<h4 css={styles.groupTitle}>
			<Stack direction="row" alignItems="end" spacing={1}>
				<span>PSK provisioners</span>
				<HelpTooltip>
					<HelpTooltipTrigger />
					<HelpTooltipContent>
						<HelpTooltipTitle>PSK provisioners</HelpTooltipTitle>
						<HelpTooltipText>
							These provisioners all use pre-shared key authentication. PSK
							provisioners are only available for the default organization.{" "}
							<Link href={docs("/")}>Learn more&hellip;</Link>
						</HelpTooltipText>
					</HelpTooltipContent>
				</HelpTooltip>
			</Stack>
		</h4>
	);
};

const styles = {
	warningBorder: (theme) => ({
		borderColor: theme.roles.warning.fill.outline,
	}),

	groupTitle: {
		fontWeight: 500,
		margin: 0,
	},

	versionPopoverTitle: (theme) => ({
		marginTop: 0,
		marginBottom: 0,
		color: theme.palette.text.primary,
		fontSize: 14,
		lineHeight: 1.5,
		fontWeight: 600,
	}),

	text: {
		marginTop: 0,
		marginBottom: 12,
	},
} satisfies Record<string, Interpolation<Theme>>;
