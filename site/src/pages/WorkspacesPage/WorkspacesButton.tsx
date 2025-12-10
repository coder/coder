import Link from "@mui/material/Link";
import type { Template } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { Loader } from "components/Loader/Loader";
import { MenuSearch } from "components/Menu/MenuSearch";
import { OverflowY } from "components/OverflowY/OverflowY";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { SearchEmpty } from "components/Search/Search";
import { ChevronDownIcon, ExternalLinkIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import { type FC, type ReactNode, useState } from "react";
import type { UseQueryResult } from "react-query";
import {
	Link as RouterLink,
	type LinkProps as RouterLinkProps,
} from "react-router";
import { cn } from "utils/cn";

type TemplatesQuery = UseQueryResult<Template[]>;

interface WorkspacesButtonProps {
	children?: ReactNode;
	templatesFetchStatus: TemplatesQuery["status"];
	templates: TemplatesQuery["data"];
}

export const WorkspacesButton: FC<WorkspacesButtonProps> = ({
	children,
	templatesFetchStatus,
	templates,
}) => {
	// Dataset should always be small enough that client-side filtering should be
	// good enough. Can swap out down the line if it becomes an issue
	const [searchTerm, setSearchTerm] = useState("");
	const processed = sortTemplatesByUsersDesc(templates ?? [], searchTerm);

	let emptyState: ReactNode;
	if (templates?.length === 0) {
		emptyState = (
			<SearchEmpty>
				No templates yet.{" "}
				<Link to="/templates" component={RouterLink}>
					Create one now.
				</Link>
			</SearchEmpty>
		);
	} else if (processed.length === 0) {
		emptyState = <SearchEmpty>No templates found</SearchEmpty>;
	}

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button size="lg">
					{children}
					<ChevronDownIcon />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="bg-surface-secondary border-surface-quaternary w-[320px]"
			>
				<MenuSearch
					value={searchTerm}
					autoFocus={true}
					onChange={setSearchTerm}
					placeholder="Type/select a workspace template"
					aria-label="Template select for workspace"
				/>

				<OverflowY maxHeight={380} className="flex flex-col py-2">
					{templatesFetchStatus === "pending" ? (
						<Loader size="sm" />
					) : (
						<>
							{processed.map((template) => (
								<WorkspaceResultsRow key={template.id} template={template} />
							))}

							{emptyState}
						</>
					)}
				</OverflowY>

				<div className="py-2 border-0 border-t border-solid border-zinc-700">
					<PopoverLink
						to="/templates"
						className="flex items-center gap-x-3 text-content-link"
					>
						<ExternalLinkIcon className="size-icon-xs" />
						<span>See all templates</span>
					</PopoverLink>
				</div>
			</PopoverContent>
		</Popover>
	);
};

interface WorkspaceResultsRowProps {
	template: Template;
}

const WorkspaceResultsRow: FC<WorkspaceResultsRowProps> = ({ template }) => {
	const getLink = useLinks();
	const templateLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);

	return (
		<PopoverLink
			to={`${templateLink}/workspace`}
			className="flex items-center gap-3"
		>
			<Avatar
				variant="icon"
				src={template.icon}
				fallback={template.display_name || template.name}
			/>

			<div className="text-content-primary flex flex-col text-sm leading-5 overflow-hidden">
				<span className="whitespace-nowrap text-ellipsis">
					{template.display_name || template.name || "[Unnamed]"}
				</span>
				<span className="text-content-secondary text-[13px]">
					{/*
					 * There are some templates that have -1 as their user count –
					 * basically functioning like a null value in JS. Can safely just
					 * treat them as if they were 0.
					 */}
					{template.active_user_count <= 0 ? "No" : template.active_user_count}{" "}
					developer
					{template.active_user_count === 1 ? "" : "s"}
				</span>
			</div>
		</PopoverLink>
	);
};

const PopoverLink: FC<RouterLinkProps> = ({
	children,
	className,
	...linkProps
}) => {
	return (
		<RouterLink
			{...linkProps}
			className={cn(
				className,
				"text-sky-500 px-4 py-2 text-sm outline-none no-underline",
				"focus:bg-surface-tertiary hover:no-underline hover:bg-zinc-800",
			)}
		>
			{children}
		</RouterLink>
	);
};

function sortTemplatesByUsersDesc(
	templates: readonly Template[],
	searchTerm: string,
) {
	const allWhitespace = /^\s+$/.test(searchTerm);
	if (allWhitespace) {
		return templates;
	}

	const termMatcher = new RegExp(searchTerm.replaceAll(/[^\w]/g, "."), "i");
	return templates
		.filter(
			(template) =>
				termMatcher.test(template.display_name) ||
				termMatcher.test(template.name),
		)
		.sort((t1, t2) => t2.active_user_count - t1.active_user_count)
		.slice(0, 10);
}
