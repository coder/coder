import { useTheme } from "@emotion/react";
import Business from "@mui/icons-material/Business";
import Person from "@mui/icons-material/Person";
import Tooltip from "@mui/material/Tooltip";
import type { HealthMessage, ProvisionerDaemon } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import type { FC } from "react";
import { createDayString } from "utils/createDayString";
import { ProvisionerTag } from "./ProvisionerTag";

interface ProvisionerProps {
	readonly provisioner: ProvisionerDaemon;
	readonly warnings?: readonly HealthMessage[];
}

export const Provisioner: FC<ProvisionerProps> = ({
	provisioner,
	warnings,
}) => {
	const theme = useTheme();
	const daemonScope = provisioner.tags.scope || "organization";
	const iconScope = daemonScope === "organization" ? <Business /> : <Person />;

	const extraTags = Object.entries(provisioner.tags).filter(
		([key]) => key !== "scope" && key !== "owner",
	);
	const isWarning = warnings && warnings.length > 0;
	return (
		<div
			key={provisioner.name}
			css={[
				{
					borderRadius: 8,
					border: `1px solid ${theme.palette.divider}`,
					fontSize: 14,
				},
				isWarning && { borderColor: theme.palette.warning.light },
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
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: 24,
						objectFit: "fill",
					}}
				>
					<div css={{ lineHeight: "160%" }}>
						<h4 css={{ fontWeight: 500, margin: 0 }}>{provisioner.name}</h4>
						<span css={{ color: theme.palette.text.secondary }}>
							<code>{provisioner.version}</code>
						</span>
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
					{extraTags.map(([key, value]) => (
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
				{provisioner.last_seen_at && (
					<span css={{ color: theme.roles.info.text }} data-chromatic="ignore">
						Last seen {createDayString(provisioner.last_seen_at)}
					</span>
				)}
			</div>
		</div>
	);
};
