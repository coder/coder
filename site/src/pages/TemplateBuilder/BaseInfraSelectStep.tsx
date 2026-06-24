import type { FC } from "react";
import { useQuery } from "react-query";
import { templateBuilderBases } from "#/api/queries/templateBuilder";
import type { TemplateBuilderBase } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { TemplateCard } from "./TemplateCard";
import type { SelectedBaseMeta } from "./wizardState";

interface BaseInfraSelectStepProps {
	selectedBaseId: string | null;
	onSelectBase: (base: SelectedBaseMeta) => void;
}

function toSelectedBaseMeta(base: TemplateBuilderBase): SelectedBaseMeta {
	return {
		id: base.id,
		name: base.name,
		iconUrl: base.icon,
		os: base.os,
		hasParameters:
			base.variables.length > 0 && base.variables.some((v) => !v.sensitive),
		hasPrerequisites: Boolean(base.prerequisites?.length),
	};
}

function detailsUrl(baseId: string): string {
	return `https://registry.coder.com/templates/${baseId}`;
}

export const BaseInfraSelectStep: FC<BaseInfraSelectStepProps> = ({
	selectedBaseId,
	onSelectBase,
}) => {
	const { data, error, isLoading } = useQuery(templateBuilderBases());

	if (isLoading) {
		return <Loader />;
	}

	if (error) {
		return <ErrorAlert error={error} />;
	}

	const bases = data?.bases ?? [];

	return (
		<div role="radiogroup" aria-label="Base infrastructure templates">
			<h2 className="text-lg font-semibold mb-1">Pick a base template</h2>
			<p className="text-sm text-content-secondary mb-4">
				Select your infrastructure foundation.
			</p>
			<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
				{bases.map((base) => (
					<TemplateCard
						key={base.id}
						name={base.name}
						description={base.description}
						iconUrl={base.icon}
						detailsUrl={detailsUrl(base.id)}
						selected={base.id === selectedBaseId}
						onSelect={() => onSelectBase(toSelectedBaseMeta(base))}
					/>
				))}
			</div>
		</div>
	);
};
