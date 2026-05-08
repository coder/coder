import {
	CircleAlertIcon,
	CircleCheckIcon,
	CircleXIcon,
	InfoIcon,
} from "lucide-react";
import { type FC, useLayoutEffect, useRef, useState } from "react";
import type { SecretRequirementStatus } from "#/api/typesGenerated";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { Link } from "#/components/Link/Link";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { docs } from "#/utils/docs";

const MANAGE_SECRETS_PATH = "/reference/cli/secret_create";

interface SecretsTableProps {
	requirements: readonly SecretRequirementStatus[];
	ownerName?: string;
}

export const secretRequirementLabel = (
	requirement: SecretRequirementStatus,
) => {
	return requirement.file || requirement.env || undefined;
};

export const hasMissingSecrets = (
	requirements: readonly SecretRequirementStatus[] | undefined,
) =>
	requirements?.some(
		(requirement) =>
			secretRequirementLabel(requirement) !== undefined &&
			!requirement.satisfied,
	) ?? false;

export const SecretsTable: FC<SecretsTableProps> = ({
	requirements,
	ownerName,
}) => {
	const manageSecretsHref = docs(MANAGE_SECRETS_PATH);
	const sortedRequirements = requirements
		.filter((requirement) => secretRequirementLabel(requirement) !== undefined)
		// Stable sort preserves backend order within each satisfied group.
		.toSorted((a, b) => {
			if (a.satisfied === b.satisfied) {
				return 0;
			}
			return a.satisfied ? 1 : -1;
		});

	if (sortedRequirements.length === 0) {
		return null;
	}

	const missingCount = sortedRequirements.filter(
		(requirement) => !requirement.satisfied,
	).length;
	const ownerCopy = ownerName
		? `for ${ownerName}'s account`
		: "for the workspace owner's account";

	return (
		<section className="flex flex-col gap-4">
			<hgroup className="flex flex-col gap-1">
				<h2 className="m-0 text-xl font-semibold text-content-primary">
					Secrets
				</h2>
				<p className="m-0 text-sm text-content-secondary">
					Secrets are injected as environment variables or files. Create or
					update these secrets with the Coder CLI {ownerCopy}.
				</p>
				{/* TODO(PLAT-102): swap for a RouterLink to /settings/secrets once
				that page lands. */}
				<Link
					href={manageSecretsHref}
					target="_blank"
					rel="noreferrer"
					showExternalIcon={false}
					className="w-fit"
				>
					View CLI documentation
				</Link>
			</hgroup>

			{missingCount >= 1 && (
				<div
					role="status"
					className="flex items-center gap-3 rounded-md border border-border border-solid bg-surface-secondary p-4 text-content-primary"
				>
					<CircleAlertIcon
						className="size-icon-sm text-content-destructive"
						aria-hidden="true"
					/>
					<p className="m-0 text-sm font-medium">
						{missingCount === 1
							? "1 required secret is missing. Use the CLI or the row action below to add it."
							: `${missingCount} required secrets are missing. Use the CLI or the row actions below to add them.`}
					</p>
				</div>
			)}

			<Table aria-label="Required secrets" className="table-fixed text-sm">
				<TableHeader className="sr-only">
					<TableRow>
						<TableHead className="w-12 px-4 py-2">Status</TableHead>
						<TableHead className="w-[38%] px-4 py-2">Secret</TableHead>
						<TableHead className="px-4 py-2">Description</TableHead>
						<TableHead className="w-40 px-4 py-2 text-right">Action</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{sortedRequirements.map((requirement) => {
						const label = secretRequirementLabel(requirement);
						if (label === undefined) {
							return null;
						}
						const key = requirement.file
							? `file:${requirement.file}`
							: `env:${requirement.env}`;

						return (
							<TableRow key={key}>
								<TableCell className="w-12 px-4 py-2">
									<StatusIcon satisfied={requirement.satisfied} />
								</TableCell>
								<TableCell className="w-[38%] overflow-hidden px-4 py-2 text-content-primary">
									<div className="truncate">{label}</div>
								</TableCell>
								<TableCell className="overflow-hidden px-4 py-2 text-content-secondary">
									<HelpMessage requirement={requirement} label={label} />
								</TableCell>
								<TableCell className="w-40 px-4 py-2 text-right">
									{!requirement.satisfied && (
										<div className="flex justify-end gap-2">
											<CopyButton
												text={secretRequirementCommand(requirement)}
												label={`Copy command for ${label}`}
												tooltipSide="left"
											/>
											<Link
												href={manageSecretsHref}
												target="_blank"
												rel="noreferrer"
												showExternalIcon={false}
												className="whitespace-nowrap self-center"
											>
												Docs
											</Link>
										</div>
									)}
								</TableCell>
							</TableRow>
						);
					})}
				</TableBody>
			</Table>
		</section>
	);
};

const secretRequirementCommand = (requirement: SecretRequirementStatus) => {
	const targetFlag = requirement.file
		? `--file ${shellQuote(requirement.file)}`
		: `--env ${shellQuote(requirement.env ?? "")}`;
	return `coder secret create SECRET_NAME ${targetFlag}`;
};

const shellQuote = (value: string) => {
	if (/^[A-Za-z0-9_./:=@%+~,-]+$/.test(value)) {
		return value;
	}
	return `'${value.replaceAll("'", "'\"'\"'")}'`;
};

const HelpMessage: FC<{
	requirement: SecretRequirementStatus;
	label: string;
}> = ({ requirement, label }) => {
	const helpMessage = requirement.help_message;
	const [isTruncated, setIsTruncated] = useState(false);
	const containerRef = useRef<HTMLDivElement>(null);
	const messageRef = useRef<HTMLSpanElement>(null);

	useLayoutEffect(() => {
		if (!helpMessage) {
			setIsTruncated(false);
			return;
		}

		const container = containerRef.current;
		const message = messageRef.current;
		if (!container || !message) {
			return;
		}

		const updateIsTruncated = () => {
			setIsTruncated(message.scrollWidth > container.clientWidth);
		};
		updateIsTruncated();

		if (typeof ResizeObserver === "undefined") {
			return;
		}

		const resizeObserver = new ResizeObserver(updateIsTruncated);
		resizeObserver.observe(container);
		resizeObserver.observe(message);
		return () => resizeObserver.disconnect();
	}, [helpMessage]);

	if (!helpMessage) {
		return null;
	}

	return (
		<div
			ref={containerRef}
			className="flex min-w-0 max-w-full items-center gap-1.5"
		>
			<span ref={messageRef} className="truncate">
				{helpMessage}
			</span>
			{isTruncated && (
				<TooltipProvider delayDuration={100}>
					<Tooltip>
						<TooltipTrigger asChild>
							<button
								type="button"
								aria-label={`Show full description for ${label}`}
								className="flex shrink-0 cursor-help items-center border-0 border-none bg-transparent p-0 text-content-secondary hover:text-content-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-content-link"
							>
								<InfoIcon className="size-icon-xs" aria-hidden="true" />
							</button>
						</TooltipTrigger>
						<TooltipContent className="max-w-sm whitespace-normal">
							{helpMessage}
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}
		</div>
	);
};

const StatusIcon: FC<{ satisfied: boolean }> = ({ satisfied }) => {
	if (satisfied) {
		return (
			<span className="inline-flex items-center text-content-success">
				<CircleCheckIcon className="size-icon-sm" aria-hidden="true" />
				<span className="sr-only">Satisfied</span>
			</span>
		);
	}

	return (
		<span className="inline-flex items-center text-content-destructive">
			<CircleXIcon className="size-icon-sm" aria-hidden="true" />
			<span className="sr-only">Missing</span>
		</span>
	);
};
