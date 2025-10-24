import type { WorkspaceAgent, WorkspaceResource } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Stack } from "components/Stack/Stack";
import { type FC, type JSX, useState } from "react";
import { ResourceCard } from "./ResourceCard";

const countAgents = (resource: WorkspaceResource) => {
	return resource.agents ? resource.agents.length : 0;
};

interface ResourcesProps {
	resources: WorkspaceResource[];
	agentRow: (agent: WorkspaceAgent, numberOfAgents: number) => JSX.Element;
}

export const Resources: FC<ResourcesProps> = ({ resources, agentRow }) => {
	const [shouldDisplayHideResources, setShouldDisplayHideResources] =
		useState(false);
	const displayResources = shouldDisplayHideResources
		? resources
		: resources
				.filter((resource) => !resource.hide)
				// Display the resources with agents first
				.sort((a, b) => countAgents(b) - countAgents(a));
	const hasHideResources = resources.some((r) => r.hide);

	return (
		<Stack direction="column" spacing={0} className="bg-surface-primary">
			{displayResources.map((resource) => (
				<ResourceCard
					key={resource.id}
					resource={resource}
					agentRow={(agent) => agentRow(agent, countAgents(resource))}
				/>
			))}
			{hasHideResources && (
				<div className="flex items-center justify-center mt-4">
					<Button
						variant="outline"
						className="rounded-full w-full max-w-[260px]"
						size="sm"
						onClick={() => setShouldDisplayHideResources((v) => !v)}
					>
						{shouldDisplayHideResources ? "Hide" : "Show hidden"} resources
						<DropdownArrow close={shouldDisplayHideResources} />
					</Button>
				</div>
			)}
		</Stack>
	);
};
