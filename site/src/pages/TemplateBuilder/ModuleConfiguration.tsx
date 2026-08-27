import { CheckIcon, InfoIcon, TrashIcon } from "lucide-react";
import type { TemplateBuilderModuleVariable } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { CollapsibleSummary } from "#/components/CollapsibleSummary/CollapsibleSummary";
import { TemplateBuilderAvatarData } from "#/pages/TemplateBuilder/TemplateBuilderAvatarData";
import {
	ConfigurationField,
	ConfigurationFieldContainer,
	type ConfigurationFieldDefinition,
} from "./ConfigurationField";

type ModuleConfigurationProps = {
	name: string;
	description: string;
	iconUrl?: string;
	detailsUrl?: string;
	onRemove?: () => void;
	fields?: ConfigurationFieldDefinition[];
	optionalFields?: ConfigurationFieldDefinition[];
	/**
	 * Sensitive variables belonging to this module. Rendered as an info note
	 * at the bottom of the module's grey card because their values are
	 * collected from the developer at workspace creation, not during template
	 * composition.
	 */
	sensitiveVariables?: TemplateBuilderModuleVariable[];
};

export const ModuleConfiguration: React.FC<ModuleConfigurationProps> = ({
	name,
	description,
	iconUrl,
	detailsUrl,
	onRemove,
	fields,
	optionalFields,
	sensitiveVariables,
}) => {
	return (
		<section className="pt-4 px-4 pb-6 rounded bg-surface-secondary">
			<header className="flex items-start gap-6 mb-6">
				<div className="flex-1">
					<TemplateBuilderAvatarData
						name={name}
						description={description}
						iconUrl={iconUrl}
						detailsUrl={detailsUrl}
					/>
				</div>
				{onRemove && (
					<Button
						variant="outline"
						size="icon"
						onClick={onRemove}
						aria-label={`Remove ${name}`}
					>
						<TrashIcon />
					</Button>
				)}
			</header>

			{fields && fields.length > 0 && (
				<ConfigurationFieldContainer>
					{fields.map((field) => (
						<ConfigurationField key={field.id} field={field} />
					))}
				</ConfigurationFieldContainer>
			)}

			{optionalFields && optionalFields.length > 0 ? (
				<CollapsibleSummary label="Additional settings" className="mt-4">
					<ConfigurationFieldContainer>
						{optionalFields.map((f) => (
							<ConfigurationField key={f.id} field={f} />
						))}
					</ConfigurationFieldContainer>
				</CollapsibleSummary>
			) : (
				<div className="text-xs text-content-secondary flex items-center gap-2 mt-4">
					<CheckIcon className="size-4" />
					No configuration required.
				</div>
			)}

			{sensitiveVariables && sensitiveVariables.length > 0 && (
				<div
					className="flex items-start gap-2 mt-4 text-xs text-content-secondary"
					data-testid="module-sensitive-variables"
				>
					<InfoIcon className="size-icon-sm shrink-0 mt-0.5" />
					<p className="m-0">
						{sensitiveVariables.map((v) => (
							<code
								key={v.name}
								className="mr-1 px-1.5 py-1 bg-surface-tertiary rounded-sm"
							>
								{v.name}
							</code>
						))}
						will be collected from developers at workspace creation.
					</p>
				</div>
			)}
		</section>
	);
};
