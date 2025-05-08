import { OpenIcon as OpenIcon } from "lucide-react";
import Link from "@mui/material/Link";
import type { Template } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { Loader } from "components/Loader/Loader";
import { MenuSearch } from "components/Menu/MenuSearch";
import { OverflowY } from "components/OverflowY/OverflowY";
import { SearchEmpty, searchStyles } from "components/Search/Search";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import { ChevronDownIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import { type FC, type ReactNode, useState } from "react";
import type { UseQueryResult } from "react-query";
import {
	Link as RouterLink,
	type LinkProps as RouterLinkProps,
} from "react-router-dom";

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

	let emptyState: ReactNode = undefined;
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
			<PopoverTrigger>
				<Button size="lg">
					{children}
					<ChevronDownIcon />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				horizontal="right"
				css={{
					".MuiPaper-root": searchStyles.content,
				}}
			>
				<MenuSearch
					value={searchTerm}
					autoFocus={true}
					onChange={setSearchTerm}
					placeholder="Type/select a workspace template"
					aria-label="Template select for workspace"
				/>

				<OverflowY
					maxHeight={380}
					css={{
						display: "flex",
						flexDirection: "column",
						paddingTop: "8px",
						paddingBottom: "8px",
					}}
				>
					{templatesFetchStatus === "loading" ? (
						<Loader size={14} />
					) : (
						<>
							{processed.map((template) => (
								<WorkspaceResultsRow key={template.id} template={template} />
							))}

							{emptyState}
						</>
					)}
				</OverflowY>

				<div
					css={(theme) => ({
						padding: "8px 0",
						borderTop: `1px solid ${theme.palette.divider}`,
					})}
				>
					<PopoverLink
						to="/templates"
						css={(theme) => ({
							display: "flex",
							alignItems: "center",
							columnGap: 12,
							color: theme.palette.primary.main,
						})}
					>
						<OpenIcon css={{ width: 14, height: 14 }} />
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
			css={{
				display: "flex",
				gap: 12,
				alignItems: "center",
			}}
		>
			<Avatar
				variant="icon"
				src={template.icon}
				fallback={template.display_name || template.name}
			/>

			<div
				css={(theme) => ({
					color: theme.palette.text.primary,
					display: "flex",
					flexDirection: "column",
					lineHeight: "140%",
					fontSize: 14,
					overflow: "hidden",
				})}
			>
				<span css={{ whiteSpace: "nowrap", textOverflow: "ellipsis" }}>
					{template.display_name || template.name || "[Unnamed]"}
				</span>
				<span
					css={(theme) => ({
						fontSize: 13,
						color: theme.palette.text.secondary,
					})}
				>
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

const PopoverLink: FC<RouterLinkProps> = ({ children, ...linkProps }) => {
	return (
		<RouterLink
			{...linkProps}
			css={(theme) => ({
				color: theme.palette.text.primary,
				padding: "8px 16px",
				fontSize: 14,
				outline: "none",
				textDecoration: "none",
				"&:focus": {
					backgroundColor: theme.palette.action.focus,
				},
				"&:hover": {
					textDecoration: "none",
					backgroundColor: theme.palette.action.hover,
				},
			})}
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
