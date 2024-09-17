import { useTheme } from "@emotion/react";
import Business from "@mui/icons-material/Business";
import Person from "@mui/icons-material/Person";
import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import type { BuildInfoResponse, ProvisionerDaemon } from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Pill } from "components/Pill/Pill";
import { type FC, useState } from "react";
import { createDayString } from "utils/createDayString";
import { ProvisionerTag } from "./ProvisionerTag";

type ProvisionerGroupType = "builtin" | "psk" | "key";

interface ProvisionerGroupProps {
	readonly buildInfo?: BuildInfoResponse;
	readonly keyName?: string;
	readonly type: ProvisionerGroupType;
	readonly provisioners: ProvisionerDaemon[];
}

export const ProvisionerGroup: FC<ProvisionerGroupProps> = ({
	buildInfo,
	keyName,
	type,
	provisioners,
}) => {
	const [provisioner] = provisioners;
	const theme = useTheme();

	const [showDetails, setShowDetails] = useState(false);

	const daemonScope = provisioner.tags.scope || "organization";
	const iconScope = daemonScope === "organization" ? <Business /> : <Person />;

	const provisionerVersion = provisioner.version;
	const allProvisionersAreSameVersion = provisioners.every(
		(provisioner) => provisioner.version === provisionerVersion,
	);
	const upToDate =
		allProvisionersAreSameVersion && buildInfo?.version === provisioner.version;
	const provisionerCount =
		provisioners.length === 1
			? "1 provisioner"
			: `${provisioners.length} provisioners`;

	const extraTags = Object.entries(provisioner.tags).filter(
		([key]) => key !== "scope" && key !== "owner",
	);

	return (
		<div
			css={{
				borderRadius: 8,
				border: `1px solid ${theme.palette.divider}`,
				fontSize: 14,
			}}
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
						display: "flex",
						gap: 12,
						flexWrap: "wrap",
					}}
				>
					{provisioners.map((provisioner) => (
						<div
							key={provisioner.id}
							css={{
								borderRadius: 8,
								border: `1px solid ${theme.palette.divider}`,
								fontSize: 14,
								padding: "12px 18px",
								width: 310,
							}}
						>
							<div css={{ lineHeight: "160%" }}>
								<h4 css={{ fontWeight: 500, margin: 0 }}>{provisioner.name}</h4>
								<span css={{ color: theme.palette.text.secondary }}>
									{type === "builtin" ? (
										<span>Built-in</span>
									) : (
										<>
											{upToDate ? "Up to date" : provisioner.version} &mdash;{" "}
											{provisioner.last_seen_at && (
												<span data-chromatic="ignore">
													Last seen {createDayString(provisioner.last_seen_at)}
												</span>
											)}
										</>
									)}
								</span>
							</div>
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
				<span>No warnings from {provisionerCount}</span>
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
