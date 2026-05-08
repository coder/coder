import {
	CircleCheckIcon,
	CircleXIcon,
	InfoIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { type FC, useLayoutEffect, useRef, useState } from "react";
import type { SecretRequirementStatus } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import {
	Table,
	TableBody,
	TableCell,
	TableRow,
} from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { docs } from "#/utils/docs";

interface SecretsTableProps {
	requirements: readonly SecretRequirementStatus[];
}

export const SecretsTable: FC<SecretsTableProps> = ({ requirements }) => {
	if (requirements.length === 0) {
		return null;
	}

	const missingCount = requirements.filter(
		(requirement) => !requirement.satisfied,
	).length;
	const sortedRequirements = [...requirements].sort((a, b) => {
		if (a.satisfied === b.satisfied) {
			return 0;
		}
		return a.satisfied ? 1 : -1;
	});

	return (
		<section className="flex flex-col gap-4">
			<hgroup className="flex flex-col gap-1">
				<h2 className="m-0 text-xl font-semibold text-content-primary">
					Secrets
				</h2>
				<p className="m-0 text-sm text-content-secondary">
					Secrets are injected as environment variables or files. Manage secrets
					in your account settings.
				</p>
				{/* TODO(PLAT-102): swap for a RouterLink to /settings/secrets once
				that page lands. */}
				<Link
					href={docs("/reference/cli/secret_create")}
					target="_blank"
					rel="noreferrer"
					showExternalIcon={false}
					className="w-fit"
				>
					Manage secrets
				</Link>
			</hgroup>

			{missingCount >= 1 && (
				<div
					role="alert"
					className="flex items-center gap-3 rounded-md border border-border border-solid bg-surface-secondary p-4 text-content-primary"
				>
					<TriangleAlertIcon
						className="size-icon-sm text-content-destructive"
						aria-hidden="true"
					/>
					<p className="m-0 text-sm font-medium">
						{missingCount === 1
							? "1 required secret is missing."
							: `${missingCount} required secrets are missing.`}
					</p>
				</div>
			)}

			<Table aria-label="Required secrets" className="table-fixed text-sm">
				<TableBody>
					{sortedRequirements.map((requirement, index) => {
						const target = secretRequirementTarget(requirement);
						let key = `unknown:${index}`;
						if (requirement.file) {
							key = `file:${requirement.file}`;
						} else if (requirement.env) {
							key = `env:${requirement.env}`;
						}

						return (
							<TableRow key={key}>
								<TableCell className="w-12 px-4 py-2">
									<StatusIcon satisfied={requirement.satisfied} />
								</TableCell>
								<TableCell className="w-[38%] overflow-hidden px-4 py-2 text-content-primary">
									<div className="truncate">{target}</div>
								</TableCell>
								<TableCell className="overflow-hidden px-4 py-2 text-content-secondary">
									<HelpMessage requirement={requirement} target={target} />
								</TableCell>
								<TableCell className="w-28 px-4 py-2 text-right">
									{!requirement.satisfied && (
										<Link
											href={docs("/reference/cli/secret_create")}
											target="_blank"
											rel="noreferrer"
											showExternalIcon={false}
											className="whitespace-nowrap"
										>
											Add secret
										</Link>
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

const secretRequirementTarget = (requirement: SecretRequirementStatus) => {
	return requirement.file || requirement.env || "Unknown secret";
};

const HelpMessage: FC<{
	requirement: SecretRequirementStatus;
	target: string;
}> = ({ requirement, target }) => {
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
								aria-label={`Show full description for ${target}`}
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
