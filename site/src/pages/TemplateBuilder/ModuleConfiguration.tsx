import { TrashIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import { CollapsibleSummary } from "#/components/CollapsibleSummary/CollapsibleSummary";
import { Link } from "#/components/Link/Link";
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
				<div className="flex flex-1 items-center gap-3 min-w-0">
					<figure className="flex items-center justify-center p-1 rounded-md size-10 shrink-0 bg-surface-secondary border border-solid border-border m-0 mb-3">
						{iconUrl ? (
							<img
								src={iconUrl}
								alt={`${name} icon`}
								className="size-7 object-contain"
							/>
						) : (
							<div className="size-7 rounded bg-surface-primary" />
						)}
					</figure>
					<div>
						<h3 className="text-md font-semibold text-content-primary my-0">
							{name}
						</h3>
						<p className="text-sm font-normal text-content-secondary inline">
							{description}
						</p>
						{detailsUrl && (
							<Link
								href={detailsUrl}
								target="_blank"
								size="sm"
								className="text-xs font-normal ml-1"
							>
								View details
							</Link>
						)}
					</div>
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

			{optionalFields && optionalFields.length > 0 && (
				<CollapsibleSummary label="Advanced settings" className="mt-4">
					<ConfigurationFieldContainer>
						{optionalFields.map((f) => (
							<ConfigurationField key={f.id} field={f} />
						))}
					</ConfigurationFieldContainer>
				</CollapsibleSummary>
			)}
		</section>
	);
};
