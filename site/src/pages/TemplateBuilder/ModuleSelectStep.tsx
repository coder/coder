import { type FC, useMemo } from "react";
import { useQuery } from "react-query";
import { templateBuilderModules } from "#/api/queries/templateBuilder";
import type {
	TemplateBuilderComposeModule,
	TemplateBuilderModule,
} from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { ModuleCard } from "./ModuleCard";
import {
	moduleHasConfigurableVars,
	type SelectedModuleMeta,
} from "./wizardState";

interface ModuleSelectStepProps {
	baseId: string;
	selectedModuleIds: string[];
	onChangeModules: (
		modules: TemplateBuilderComposeModule[],
		meta: SelectedModuleMeta[],
	) => void;
}

function toMeta(m: TemplateBuilderModule): SelectedModuleMeta {
	return {
		id: m.id,
		name: m.display_name,
		iconUrl: m.icon,
		hasConfigurableVars: moduleHasConfigurableVars(m),
	};
}

// TODO add this to the API response so we don't have to construct it manually here.
function moduleDetailsUrl(moduleId: string): string {
	return `https://registry.coder.com/modules/${moduleId}`;
}

interface ModuleConflict {
	moduleA: TemplateBuilderModule;
	moduleB: TemplateBuilderModule;
}

const ConflictWarning: FC<ModuleConflict> = ({ moduleA, moduleB }) => {
	return (
		<div>
			<code className="text-content-secondary bg-surface-tertiary mr-1 px-1.5 py-1 rounded-sm">
				{moduleA.display_name}
			</code>{" "}
			and{" "}
			<code className="text-content-secondary bg-surface-tertiary mx-1 px-1.5 py-1 rounded-sm">
				{moduleB.display_name}
			</code>
			are conflicting modules. You can still continue, but you need to remove
			one of the conflicting modules before publishing the template.
		</div>
	);
};

export const ModuleSelectStep: FC<ModuleSelectStepProps> = ({
	baseId,
	selectedModuleIds,
	onChangeModules,
}) => {
	const { data, error, isLoading } = useQuery(templateBuilderModules(baseId));

	const selectedSet = useMemo(
		() => new Set(selectedModuleIds),
		[selectedModuleIds],
	);

	const modules = data?.modules ?? [];

	const conflicts = useMemo<ModuleConflict[]>(() => {
		const warnings: string[] = [];
		// Loop through the selected modules and check for conflicts. We sort the
		// pair of conflicting module IDs alphabetically and join them with a "+"
		// to create a unique identifier for the pair.
		for (const id of selectedSet) {
			const m = modules.find((mod) => mod.id === id);
			if (!m) continue;
			for (const conflictId of m.conflicts_with) {
				if (selectedSet.has(conflictId)) {
					const pair = [id, conflictId].sort().join("+");
					if (!warnings.includes(pair)) {
						warnings.push(pair);
					}
				}
			}
		}
		// take the computed warnings and return the actual modules that have
		// conflicts so we can display their names in the UI.
		return warnings.map((pair) => {
			const [a, b] = pair.split("+");
			const moduleA = modules.find((m) => m.id === a)!;
			const moduleB = modules.find((m) => m.id === b)!;
			return { moduleA, moduleB };
		});
	}, [selectedSet, modules]);

	if (isLoading) {
		return <Loader />;
	}

	if (error) {
		return <ErrorAlert error={error} />;
	}

	const handleToggle = (target: TemplateBuilderModule) => {
		const isSelected = selectedSet.has(target.id);
		let nextIds: string[];
		if (isSelected) {
			nextIds = selectedModuleIds.filter((id) => id !== target.id);
		} else {
			nextIds = [...selectedModuleIds, target.id];
		}

		const modulesById = new Map(modules.map((m) => [m.id, m]));
		const nextModules: TemplateBuilderComposeModule[] = nextIds.map((id) => ({
			id,
		}));
		const nextMeta: SelectedModuleMeta[] = nextIds
			.map((id) => modulesById.get(id))
			.filter((m): m is TemplateBuilderModule => m != null)
			.map(toMeta);

		onChangeModules(nextModules, nextMeta);
	};

	return (
		<div>
			<h2 className="text-lg font-semibold mb-1">Select modules</h2>
			<p className="text-sm text-content-secondary mb-4">
				Add functionality to your template.
			</p>

			<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
				{modules.map((m) => (
					<ModuleCard
						key={m.id}
						name={m.display_name}
						description={m.description}
						iconUrl={m.icon}
						detailsUrl={moduleDetailsUrl(m.id)}
						selected={selectedSet.has(m.id)}
						onSelect={() => handleToggle(m)}
					/>
				))}
			</div>

			{conflicts.length > 0 && (
				<Alert severity="warning" prominent className="mt-6">
					<AlertTitle>Conflicting modules selected</AlertTitle>
					<AlertDescription>
						{conflicts.map(({ moduleA, moduleB }) => (
							<ConflictWarning
								key={moduleA.id + moduleB.id}
								moduleA={moduleA}
								moduleB={moduleB}
							/>
						))}
					</AlertDescription>
				</Alert>
			)}
		</div>
	);
};
