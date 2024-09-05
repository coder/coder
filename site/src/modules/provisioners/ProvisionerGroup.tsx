import { useTheme } from "@emotion/react";
import Business from "@mui/icons-material/Business";
import Person from "@mui/icons-material/Person";
import Tooltip from "@mui/material/Tooltip";
import type { HealthMessage, ProvisionerDaemon } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import type { FC } from "react";
import { createDayString } from "utils/createDayString";
import { ProvisionerTag } from "./ProvisionerTag";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { buildInfo } from "api/queries/buildInfo";
import { useQuery } from "react-query";
import ArrowDownward from "@mui/icons-material/ArrowDownward";

type ProvisionerGroupType = "builtin" | "psk" | "key";

interface ProvisionerGroupProps {
	readonly keyName?: string;
	readonly type: ProvisionerGroupType;
	readonly provisioners: ProvisionerDaemon[];
	readonly warnings?: readonly HealthMessage[];
}

export const ProvisionerGroup: FC<ProvisionerGroupProps> = ({
	keyName,
	type,
	provisioners,
	warnings,
}) => {
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));

	const [provisioner] = provisioners;
	const theme = useTheme();
	const daemonScope = provisioner.tags.scope || "organization";
	const iconScope = daemonScope === "organization" ? <Business /> : <Person />;

	const provisionerVersion = provisioner.version;
	const allProvisionersAreSameVersion = provisioners.every(
		(provisioner) => provisioner.version === provisionerVersion,
	);
	const upToDate =
		allProvisionersAreSameVersion && buildInfoQuery.data?.version;
	const protocolUpToDate =
		allProvisionersAreSameVersion &&
		buildInfoQuery?.data?.provisioner_api_version === provisioner.api_version;
	const provisionerCount =
		provisioners.length === 1
			? "1 provisioner"
			: `${provisioners.length} provisioners`;

	const extraTags = Object.entries(provisioner.tags).filter(
		([key]) => key !== "scope" && key !== "owner",
	);
	const isWarning = warnings && warnings.length > 0;
	return (
		<div
			css={[
				{
					borderRadius: 8,
					border: `1px solid ${theme.palette.divider}`,
					fontSize: 14,
				},
				isWarning && { borderColor: theme.roles.warning.outline },
			]}
		>
			<header
				css={{
					padding: 24,
					display: "flex",
					alignItems: "center",
					justifyContenxt: "space-between",
					gap: 24,
				}}
			>
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: 24,
						objectFit: "fill",
					}}
				>
					{type === "builtin" && (
						<div css={{ lineHeight: "160%" }}>
							<h4 css={{ fontWeight: 500, margin: 0 }}>
								Built-in provisioners
							</h4>
							<span css={{ color: theme.palette.text.secondary }}>
								{provisionerCount} &mdash; Built-in
							</span>
						</div>
					)}
					{type === "psk" && (
						<div css={{ lineHeight: "160%" }}>
							<h4 css={{ fontWeight: 500, margin: 0 }}>PSK provisioners</h4>
							<span css={{ color: theme.palette.text.secondary }}>
								{provisionerCount} &mdash;{" "}
								{allProvisionersAreSameVersion ? (
									<code>{provisionerVersion}</code>
								) : (
									<span>Multiple versions</span>
								)}
							</span>
						</div>
					)}
					{type === "key" && (
						<div css={{ lineHeight: "160%" }}>
							<h4 css={{ fontWeight: 500, margin: 0 }}>
								Key group &ndash; {keyName}
							</h4>
							<span css={{ color: theme.palette.text.secondary }}>
								{provisionerCount} &mdash;{" "}
								{allProvisionersAreSameVersion ? (
									<code>{provisionerVersion}</code>
								) : (
									<span>Multiple versions</span>
								)}
							</span>
						</div>
					)}
				</div>
				<div
					css={{
						marginLeft: "auto",
						display: "flex",
						flexWrap: "wrap",
						gap: 12,
					}}
				>
					<Tooltip title="Scope">
						<Pill size="lg" icon={iconScope}>
							<span
								css={{
									":first-letter": { textTransform: "uppercase" },
								}}
							>
								{daemonScope}
							</span>
						</Pill>
					</Tooltip>
					{type === "key" &&
						extraTags.map(([key, value]) => (
							<ProvisionerTag key={key} tagName={key} tagValue={value} />
						))}
				</div>
			</header>

			<div
				css={{
					borderTop: `1px solid ${theme.palette.divider}`,
					display: "flex",
					alignItems: "center",
					justifyContent: "space-between",
					padding: "8px 24px",
					fontSize: 12,
					color: theme.palette.text.secondary,
				}}
			>
				{warnings && warnings.length > 0 ? (
					<div css={{ display: "flex", flexDirection: "column" }}>
						{warnings.map((warning) => (
							<span key={warning.code}>{warning.message}</span>
						))}
					</div>
				) : (
					<span>No warnings</span>
				)}
				<span
					css={{
						display: "flex",
						alignItems: "center",
						gap: 4,
						color: theme.roles.info.text,
					}}
				>
					Show provisioner details{" "}
					<ArrowDownward fontSize="small" color="inherit" />
				</span>
			</div>
		</div>
	);
};
