import type {
	ProvisionerDaemon,
	ProvisionerDaemonStatus,
} from "api/typesGenerated";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import { TableCell, TableRow } from "components/Table/Table";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { JobStatusIndicator } from "modules/provisioners/JobStatusIndicator";
import {
	ProvisionerTag,
	ProvisionerTags,
	ProvisionerTruncateTags,
} from "modules/provisioners/ProvisionerTags";
import { useState, type FC } from "react";
import { cn } from "utils/cn";
import { relativeTime } from "utils/time";
import { ProvisionerVersion } from "./ProvisionerVersion";
import { ProvisionerKey } from "pages/OrganizationSettingsPage/OrganizationProvisionersPage/ProvisionerKey";

const variantByStatus: Record<
	ProvisionerDaemonStatus,
	StatusIndicatorProps["variant"]
> = {
	idle: "success",
	busy: "pending",
	offline: "inactive",
};

type ProvisionerRowProps = {
	provisioner: ProvisionerDaemon;
	buildVersion: string | undefined;
};

export const ProvisionerRow: FC<ProvisionerRowProps> = ({
	provisioner,
	buildVersion,
}) => {
	const [isOpen, setIsOpen] = useState(false);

	return (
		<>
			<TableRow>
				<TableCell>
					<button
						className={cn([
							"flex items-center gap-1 p-0 bg-transparent border-0 text-inherit text-xs cursor-pointer",
							"transition-colors hover:text-content-primary font-medium whitespace-nowrap",
							isOpen && "text-content-primary",
						])}
						type="button"
						onClick={() => {
							setIsOpen((v) => !v);
						}}
					>
						{isOpen ? (
							<ChevronDownIcon className="size-icon-sm p-0.5" />
						) : (
							<ChevronRightIcon className="size-icon-sm p-0.5" />
						)}
						<span className="sr-only">({isOpen ? "Hide" : "Show more"})</span>
						{provisioner.name}
					</button>
				</TableCell>
				<TableCell>
					{provisioner.key_name && (
						<ProvisionerKey name={provisioner.key_name} />
					)}
				</TableCell>
				<TableCell>
					<ProvisionerVersion
						buildVersion={buildVersion}
						provisionerVersion={provisioner.version}
					/>
				</TableCell>
				<TableCell>
					{provisioner.status && (
						<StatusIndicator
							size="sm"
							variant={variantByStatus[provisioner.status]}
						>
							<StatusIndicatorDot />
							<span className="block first-letter:uppercase">
								{provisioner.status}
							</span>
						</StatusIndicator>
					)}
				</TableCell>
				<TableCell>
					<ProvisionerTruncateTags tags={provisioner.tags} />
				</TableCell>
				<TableCell>
					{provisioner.last_seen_at ? (
						<span className="block first-letter:uppercase">
							{relativeTime(new Date(provisioner.last_seen_at))}
						</span>
					) : (
						"Never"
					)}
				</TableCell>
			</TableRow>

			{isOpen && (
				<TableRow>
					<TableCell colSpan={999} className="p-4 border-t-0">
						<dl
							className={cn([
								"text-xs text-content-secondary",
								"m-0 grid grid-cols-[auto_1fr] gap-x-4 items-center",
								"[&_dd]:text-content-primary [&_dd]:font-mono [&_dd]:leading-[22px] [&_dt]:font-medium",
							])}
						>
							<dt>Last seen:</dt>
							<dd>{provisioner.last_seen_at}</dd>

							<dt>Creation time:</dt>
							<dd>{provisioner.created_at}</dd>

							<dt>Version:</dt>
							<dd>
								{provisioner.version === buildVersion
									? "up to date"
									: "outdated"}
							</dd>

							<dt>Tags:</dt>
							<dd>
								<ProvisionerTags>
									{Object.entries(provisioner.tags).map(([key, value]) => (
										<ProvisionerTag key={key} label={key} value={value} />
									))}
								</ProvisionerTags>
							</dd>

							<div className="h-6 w-full col-span-2" />

							{provisioner.current_job && (
								<>
									<dt>Current job:</dt>
									<dd>{provisioner.current_job.id}</dd>

									<dt>Current job status:</dt>
									<dd>
										<JobStatusIndicator
											status={provisioner.current_job.status}
										/>
									</dd>
								</>
							)}

							{provisioner.previous_job && (
								<>
									<dt>Previous job:</dt>
									<dd>{provisioner.previous_job.id}</dd>

									<dt>Previous job status:</dt>
									<dd>
										<JobStatusIndicator
											status={provisioner.previous_job.status}
										/>
									</dd>
								</>
							)}
						</dl>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};
