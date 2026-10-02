import { ExternalLinkIcon } from "lucide-react";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import type { ContextMenuItem } from "#/components/ContextMenu/ContextMenu";
import type { DropdownMenuItem } from "#/components/DropdownMenu/DropdownMenu";
import { originRepoLabel } from "../utils/originRepoLabel";
import { prNumber } from "../utils/pullRequest";
import { getPRIconConfig } from "./ChatsSidebar/tree/statusConfig";

/** Menu content for PR links. The height fits five rows before it scrolls. */
export const prMenuContentClassName =
	"max-h-50 w-72 overflow-auto [&_[role=menuitem]]:text-[13px]";

type PRMenuLinksProps = {
	readonly prStatuses: readonly ChatDiffStatus[];
	readonly Item: typeof DropdownMenuItem | typeof ContextMenuItem;
};

export const PRMenuLinks: React.FC<PRMenuLinksProps> = ({
	prStatuses,
	Item,
}) => {
	// PR numbers repeat across repositories, so entries name the
	// repository when the PRs span several.
	const hasMultipleOrigins =
		new Set(prStatuses.map((status) => status.remote_origin).filter(Boolean))
			.size > 1;

	return prStatuses.map((status) => {
		const config = getPRIconConfig(status);
		const repo = hasMultipleOrigins && originRepoLabel(status.remote_origin);
		const number = prNumber(status);
		const title = status.pull_request_title.trim();

		return (
			<Item key={`${status.remote_origin}/${status.git_branch}`} asChild>
				<a href={status.url} target="_blank" rel="noreferrer">
					{config && (
						<config.icon
							role="img"
							aria-label={config.label}
							className={config.className}
						/>
					)}
					<span className="shrink-0 text-content-primary">
						{repo && `${repo} · `}
						{number ? `PR #${number}` : "Pull request"}
					</span>
					{title && (
						<span className="min-w-0 truncate font-normal">{title}</span>
					)}
					<ExternalLinkIcon className="ml-auto size-3.5!" />
				</a>
			</Item>
		);
	});
};
