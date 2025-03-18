import type { Template } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { FC } from "react";
import { Link } from "react-router-dom";

interface WorkspacesEmptyProps {
	isUsingFilter: boolean;
	templates?: Template[];
	canCreateTemplate: boolean;
}

export const WorkspacesEmpty: FC<WorkspacesEmptyProps> = ({
	isUsingFilter,
	templates,
	canCreateTemplate,
}) => {
	const getLink = useLinks();

	const totalFeaturedTemplates = 6;
	const featuredTemplates = templates?.slice(0, totalFeaturedTemplates);
	const defaultTitle = "Create a workspace";
	const defaultMessage =
		"A workspace is your personal, customizable development environment.";
	const defaultImage = (
		<div className="max-w-[50%] h-[272px] overflow-hidden mt-12 opacity-85">
			<img src="/featured/workspaces.webp" alt="" className="max-w-full" />
		</div>
	);

	if (isUsingFilter) {
		return <TableEmpty message="No results matched your search" />;
	}

	if (templates && templates.length === 0 && canCreateTemplate) {
		return (
			<TableEmpty
				message={defaultTitle}
				description={`${defaultMessage} To create a workspace, you first need to create a template.`}
				cta={
					<Button asChild>
						<Link to="/templates">Go to templates</Link>
					</Button>
				}
				className="pb-0"
				image={defaultImage}
			/>
		);
	}

	if (templates && templates.length === 0 && !canCreateTemplate) {
		return (
			<TableEmpty
				message={defaultTitle}
				description={`${defaultMessage} There are no templates available, but you will see them here once your admin adds them.`}
				className="pb-0"
				image={defaultImage}
			/>
		);
	}

	return (
		<TableEmpty
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
								<div className="flex-shrink-0 pt-1">
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
										className="text-sm text-gray-400 leading-[1.4] m-0 pt-1 break-words"
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
