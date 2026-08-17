import { ExternalLinkIcon } from "lucide-react";
import { type FC, type ReactNode, useMemo } from "react";
import type { UseQueryResult } from "react-query";
import { Link as RouterLink, useNavigate } from "react-router";
import type { Template } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import { linkToTemplate, useLinks } from "#/modules/navigation";

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
	const navigate = useNavigate();
	const getLink = useLinks();

	// Dataset should always be small enough that client-side filtering should be
	// good enough. Can swap out down the line if it becomes an issue
	const sortedTemplates = useMemo(
		() => sortTemplatesByUsersDesc(templates ?? []),
		[templates],
	);

	return (
		<Combobox
			onValueChange={(templateId) => {
				if (!templateId || !templates) {
					return;
				}
				const template = templates.find((t) => t.id === templateId);
				if (!template) {
					return;
				}
				navigate(
					`${getLink(linkToTemplate(template.organization_name, template.name))}/workspace`,
				);
			}}
		>
			<ComboboxTrigger asChild>
				<Button size="lg">
					{children}
					<ChevronDownIcon />
				</Button>
			</ComboboxTrigger>
			<ComboboxContent
				align="end"
				className="w-[320px] max-w-[320px] overflow-hidden bg-surface-secondary border-surface-quaternary"
			>
				<ComboboxInput
					placeholder="Type/select a workspace template"
					aria-label="Template select for workspace"
				/>
				<ComboboxList
					// Cap the list to the popover's available height minus the search
					// input and footer. A nested max-height (e.g. 380px) larger than
					// the remaining space clips the footer on short viewports.
					style={{
						maxHeight:
							"min(380px, calc(var(--radix-popper-available-height) - 8rem))",
					}}
				>
					{templatesFetchStatus === "pending" ? (
						<div className="flex items-center justify-center py-4">
							<Spinner size="sm" loading />
						</div>
					) : (
						sortedTemplates.map((template) => (
							<ComboboxItem
								key={template.id}
								value={template.id}
								keywords={[template.display_name, template.name]}
								className="px-4 data-[selected=true]:bg-surface-tertiary font-normal gap-3 [&>svg:last-child]:hidden"
							>
								<Avatar
									variant="icon"
									size="sm"
									src={template.icon}
									fallback={template.display_name || template.name}
								/>
								<div className="flex min-w-0 flex-1 flex-col overflow-hidden leading-[140%]">
									<span className="truncate text-content-primary">
										{template.display_name || template.name || "[Unnamed]"}
									</span>
									<span className="truncate text-xs font-normal text-content-secondary">
										{activeDeveloperLabel(template.active_user_count)}
									</span>
								</div>
							</ComboboxItem>
						))
					)}
				</ComboboxList>
				{templatesFetchStatus !== "pending" &&
					(templates?.length === 0 ? (
						<ComboboxEmpty>
							No templates yet.{" "}
							<Link asChild showExternalIcon={false}>
								<RouterLink to="/templates">Create one now.</RouterLink>
							</Link>
						</ComboboxEmpty>
					) : (
						<ComboboxEmpty>No templates found</ComboboxEmpty>
					))}
				<div className="shrink-0 border-0 border-t border-solid border-border py-1">
					<RouterLink
						to="/templates"
						className="flex items-center gap-3 px-4 py-2 text-sm text-content-link no-underline outline-none hover:bg-surface-tertiary hover:no-underline focus:bg-surface-tertiary"
					>
						<ExternalLinkIcon className="size-icon-xs" />
						<span>See all templates</span>
					</RouterLink>
				</div>
			</ComboboxContent>
		</Combobox>
	);
};

function activeDeveloperLabel(count: number): string {
	// Some templates report -1 as their user count, which is treated like a
	// null. Treat those the same as 0.
	if (count <= 0) {
		return "No developers";
	}
	return `${count} developer${count === 1 ? "" : "s"}`;
}

function sortTemplatesByUsersDesc(templates: readonly Template[]) {
	return templates.toSorted(
		(t1, t2) => t2.active_user_count - t1.active_user_count,
	);
}
