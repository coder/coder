import { CheckIcon, TrashIcon } from "lucide-react";
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
};

export const ModuleConfiguration: React.FC<ModuleConfigurationProps> = ({
	name,
	description,
	iconUrl,
	detailsUrl,
	onRemove,
	fields,
	optionalFields,
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
				<div className="text-sm text-content-secondary flex items-center gap-2 mt-4">
					<CheckIcon className="size-4" />
					No configuration required.
				</div>
			)}
		</section>
	);
};
