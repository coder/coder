import { CircleAlertIcon, CircleCheckIcon, CircleXIcon } from "lucide-react";
import type { FC } from "react";
import type { SecretRequirementStatus } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
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
					update these secrets with the Coder CLI {ownerCopy}.{" "}
					{/* TODO(PLAT-102): swap for a RouterLink to /settings/secrets once
					that page lands. */}
					<Link href={manageSecretsHref} target="_blank" rel="noreferrer">
						View CLI documentation
					</Link>
				</p>
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
							? "1 required secret is missing."
							: `${missingCount} required secrets are missing.`}
					</p>
				</div>
			)}

			<Table aria-label="Required secrets" className="table-fixed text-sm">
				<colgroup>
					<col className="w-[45%]" />
					<col />
					<col className="w-28" />
				</colgroup>
				<TableHeader className="sr-only">
					<TableRow>
						<TableHead className="px-4 py-2">Secret</TableHead>
						<TableHead className="px-4 py-2">Description</TableHead>
						<TableHead className="px-4 py-2 text-right">Action</TableHead>
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
								<TableCell className="overflow-hidden px-4 py-2 text-content-primary">
									<div className="flex min-w-0 items-center gap-3">
										<StatusIcon satisfied={requirement.satisfied} />
										<div className="min-w-0 truncate" title={label}>
											{label}
										</div>
									</div>
								</TableCell>
								<TableCell className="overflow-hidden px-4 py-2 text-content-secondary">
									<HelpMessage requirement={requirement} />
								</TableCell>
								<TableCell className="px-4 py-2 text-right">
									{!requirement.satisfied && (
										<Link
											href={manageSecretsHref}
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

const HelpMessage: FC<{
	requirement: SecretRequirementStatus;
}> = ({ requirement }) => {
	const helpMessage = requirement.help_message;
	if (!helpMessage) {
		return null;
	}

	return (
		<span className="block truncate" title={helpMessage}>
			{helpMessage}
		</span>
	);
};

const StatusIcon: FC<{ satisfied: boolean }> = ({ satisfied }) => {
	if (satisfied) {
		return (
			<span className="inline-flex shrink-0 items-center text-content-success">
				<CircleCheckIcon className="size-icon-sm" aria-hidden="true" />
				<span className="sr-only">Satisfied</span>
			</span>
		);
	}

	return (
		<span className="inline-flex shrink-0 items-center text-content-destructive">
			<CircleXIcon className="size-icon-sm" aria-hidden="true" />
			<span className="sr-only">Missing</span>
		</span>
	);
};
