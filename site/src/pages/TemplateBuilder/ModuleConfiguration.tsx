import { TrashIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import {
	ConfigurationField,
	type ConfigurationFieldDefinition,
} from "./ConfigurationField";

type ModuleConfigurationProps = {
	name: string;
	description: string;
	iconUrl?: string;
	detailsUrl?: string;
	onRemove?: () => void;
	fields?: ConfigurationFieldDefinition[];
};

export const ModuleConfiguration: React.FC<ModuleConfigurationProps> = ({
	name,
	description,
	iconUrl,
	detailsUrl,
	onRemove,
	fields,
}) => {
	return (
		<div className="flex flex-col gap-6 pt-4 px-4 pb-6 rounded bg-surface-secondary">
			<div className="flex items-start gap-6">
				<div className="flex flex-1 items-center gap-3 min-w-0">
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
					<div className="flex flex-col flex-1 min-w-0">
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
			</div>

			{fields && fields.length > 0 && (
				<div className="grid grid-cols-1 md:grid-cols-2 gap-6 items-start">
					{fields.map((field) => (
						<ConfigurationField key={field.id} field={field} />
					))}
				</div>
			)}
		</div>
	);
};
