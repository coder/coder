import { Link } from "#/components/Link/Link";
import {
	ConfigurationField,
	type ConfigurationFieldDefinition,
} from "./ConfigurationField";

type TemplateConfigurationProps = {
	name: string;
	description: string;
	iconUrl?: string;
	detailsUrl?: string;
	fields?: ConfigurationFieldDefinition[];
};

export const TemplateConfiguration: React.FC<TemplateConfigurationProps> = ({
	name,
	description,
	iconUrl,
	detailsUrl,
	fields,
}) => {
	return (
		<div className="flex flex-col gap-6 pt-4 px-4 pb-6 rounded bg-surface-secondary">
			<div className="flex flex-col gap-3">
				<div className="flex items-center justify-center p-1 rounded-md size-10 shrink-0 bg-surface-secondary border border-solid border-border">
					{iconUrl ? (
						<img
							src={iconUrl}
							alt={`${name} icon`}
							className="size-7 object-contain"
						/>
					) : (
						<div className="size-7 rounded bg-surface-primary" />
					)}
				</div>
				<div className="flex flex-col">
					<div className="text-sm font-semibold text-content-primary">
						{name}
					</div>
					<div className="flex items-center flex-wrap">
						<span className="text-xs font-normal text-content-secondary">
							{description}
						</span>
						{detailsUrl && (
							<Link
								href={detailsUrl}
								target="_blank"
								rel="noreferrer"
								size="sm"
								className="text-xs font-normal ml-1"
							>
								View details
							</Link>
						)}
					</div>
				</div>
			</div>

			{fields && fields.length > 0 && (
				<div className="flex flex-col gap-6">
					{fields.map((field) => (
						<ConfigurationField key={field.id} field={field} />
					))}
				</div>
			)}
		</div>
	);
};
