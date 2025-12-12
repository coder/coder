import { useTheme } from "@emotion/react";
import type { HealthMessage, ProvisionerDaemon } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { Building2Icon, UserIcon } from "lucide-react";
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
	const iconScope =
		daemonScope === "organization" ? (
			<Building2Icon className="size-icon-sm" />
		) : (
			<UserIcon className="size-icon-sm" />
		);

	const extraTags = Object.entries(provisioner.tags).filter(
		([key]) => key !== "scope" && key !== "owner",
	);
	const isWarning = warnings && warnings.length > 0;
	return (
		<div
			key={provisioner.name}
			className="rounded-lg text-sm leading-none"
			css={[
				{
					border: `1px solid ${theme.palette.divider}`,
				},
				isWarning && { borderColor: theme.palette.warning.light },
			]}
		>
			<header className="p-6 flex items-center justify-between gap-6">
				<div className="flex items-center gap-6 object-fill">
					<div className="leading-[160%]">
						<h4 className="font-semibold m-0">{provisioner.name}</h4>
						<span css={{ color: theme.palette.text.secondary }}>
							<code>{provisioner.version}</code>
						</span>
					</div>
				</div>
				<div className="ml-auto flex flex-wrap gap-3 justify-end">
					<Tooltip>
						<TooltipTrigger asChild>
							<Pill size="lg" icon={iconScope}>
								<span className="first-letter:uppercase">{daemonScope}</span>
							</Pill>
						</TooltipTrigger>
						<TooltipContent side="bottom">Scope</TooltipContent>
					</Tooltip>
					{extraTags.map(([key, value]) => (
						<ProvisionerTag key={key} tagName={key} tagValue={value} />
					))}
				</div>
			</header>

			<div
				css={{
					borderTop: `1px solid ${theme.palette.divider}`,
					color: theme.palette.text.secondary,
				}}
				className="flex items-center justify-between py-2 px-6 text-xs leading-loose"
			>
				{warnings && warnings.length > 0 ? (
					<div className="flex flex-col">
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
