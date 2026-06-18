import type { ReactNode } from "react";
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
	children?: ReactNode;
};

export const TemplateConfiguration: React.FC<TemplateConfigurationProps> = ({
	name,
	description,
	iconUrl,
	detailsUrl,
	fields,
	children,
}) => {
	return (
		<section className="pt-4 px-4 pb-6 rounded bg-surface-secondary">
			<header className="mb-6">
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
					<h3 className="text-md font-semibold text-content-primary">{name}</h3>
					<p className="text-sm font-normal text-content-secondary inline">
						{description}
					</p>
					{detailsUrl && (
						<Link
							href={detailsUrl}
							target="_blank"
							rel="noreferrer"
							size="sm"
							className="text-sm font-normal ml-1"
						>
							View details
						</Link>
					)}
				</div>
			</header>

			{fields && fields.length > 0 && (
				<div className="space-y-6">
					{fields.map((field) => (
						<ConfigurationField key={field.id} field={field} />
					))}
				</div>
			)}
			{children}
		</section>
	);
};
