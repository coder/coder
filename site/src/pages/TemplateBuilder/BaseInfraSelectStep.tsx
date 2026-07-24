import type { FC } from "react";
import { useQuery } from "react-query";
import { templateBuilderBases } from "#/api/queries/templateBuilder";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import {
	TemplateBuilderSubtitle,
	TemplateBuilderTitle,
} from "#/pages/TemplateBuilder/TemplateBuilderHeader";
import { TemplateCard } from "./TemplateCard";
import { type SelectedBaseMeta, toSelectedBaseMeta } from "./wizardState";

interface BaseInfraSelectStepProps {
	selectedBaseId: string | null;
	onSelectBase: (base: SelectedBaseMeta) => void;
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
			<TemplateBuilderTitle>Pick a base template</TemplateBuilderTitle>
			<TemplateBuilderSubtitle>
				Select your infrastructure foundation.
			</TemplateBuilderSubtitle>

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
