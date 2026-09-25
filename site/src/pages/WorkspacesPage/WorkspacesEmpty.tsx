import type { FC } from "react";
import { Link } from "react-router";
import type { Template } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { linkToTemplate, useLinks } from "#/modules/navigation";

type WorkspacesEmptyProps = {
	isUsingFilter: boolean;
	onClearFilter: () => void;
	templates?: Template[];
	canCreateTemplate: boolean;
	canCreateWorkspace: boolean;
};

export const WorkspacesEmpty: FC<WorkspacesEmptyProps> = ({
	isUsingFilter,
	onClearFilter,
	templates,
	canCreateTemplate,
	canCreateWorkspace,
}) => {
	const getLink = useLinks();

	const totalFeaturedTemplates = 6;
	const featuredTemplates = templates?.slice(0, totalFeaturedTemplates);
	const defaultTitle = "Create a workspace";
	const defaultMessage =
		"A workspace is your personal, customizable development environment.";

	if (isUsingFilter) {
		return (
			<EmptyState
				message="No workspaces match your search."
				cta={
					<Button variant="outline" onClick={onClearFilter}>
						Clear all
					</Button>
				}
			/>
		);
	}

	if (!canCreateWorkspace) {
		return (
			<EmptyState
				message="No workspaces"
				description="You don't have permission to create workspaces. Contact your administrator if you need workspace access."
			/>
		);
	}

	if (templates && templates.length === 0 && canCreateTemplate) {
		return (
			<EmptyState
				message={defaultTitle}
				description={`${defaultMessage} To create a workspace, you first need to create a template.`}
				cta={
					<Button asChild>
						<Link to="/templates/new/builder">Create a template</Link>
					</Button>
				}
			/>
		);
	}

	if (templates && templates.length === 0 && !canCreateTemplate) {
		return (
			<div className="flex flex-col items-center px-10 pt-12 pb-20 text-center">
				<div className="relative mb-6 w-[360px] max-w-full aspect-[360/225]">
					<div className="absolute inset-x-0 top-0 h-[88.9%] overflow-hidden rounded-[10px]">
						<div
							aria-hidden="true"
							className="absolute inset-0 bg-(image:--supergraphic-square-url) bg-size-[190%_auto] bg-position-[14%_0%] bg-no-repeat"
						/>
						<div
							aria-hidden="true"
							className="absolute inset-0 bg-white/10 backdrop-blur-[2.64px]"
						/>
					</div>
					<img
						src="/media/waiting-on-templates.svg"
						alt=""
						className="absolute inset-0 h-full w-full"
					/>
				</div>
				<h3 className="m-0 font-medium text-content-primary text-lg">
					Waiting on templates
				</h3>
				<p className="mt-2 max-w-[360px] text-content-secondary text-sm">
					Workspaces are personal, customizable environments built from
					templates. Once your admin adds one, it'll show up here and you’ll be
					ready to start.
				</p>
			</div>
		);
	}

	return (
		<EmptyState
			message={defaultTitle}
			description={`${defaultMessage} Select one template below to start.`}
			cta={
				<div>
					<div className="flex flex-wrap gap-4 mb-6 justify-center max-w-[800px]">
						{featuredTemplates?.map((t) => (
							<Link
								key={t.id}
								to={`${getLink(
									linkToTemplate(t.organization_name, t.name),
								)}/workspace`}
								className="w-[320px] p-4 rounded-md border border-solid border-surface-quaternary text-left flex gap-4 no-underline text-inherit hover:bg-surface-grey"
							>
								<div className="shrink-0 pt-1">
									<Avatar variant="icon" src={t.icon} fallback={t.name} />
								</div>

								<div className="w-full min-w-0">
									<h4 className="text-sm font-semibold m-0 overflow-hidden truncate whitespace-nowrap">
										{t.display_name || t.name}
									</h4>

									<p
										// We've had users plug URLs directly into the
										// descriptions, when those URLS have no hyphens or other
										// easy semantic breakpoints. Need to set this to ensure
										// those URLs don't break outside their containing boxes
										className="text-sm text-gray-400 leading-[1.4] m-0 pt-1 wrap-break-word"
									>
										{t.description}
									</p>
								</div>
							</Link>
						))}
					</div>

					{templates && templates.length > totalFeaturedTemplates && (
						<Button asChild>
							<Link to="/templates">See all templates</Link>
						</Button>
					)}
				</div>
			}
		/>
	);
};
