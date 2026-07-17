import type { PropsWithChildren } from "react";
import { TemplateBuilderAvatarData } from "#/pages/TemplateBuilder/TemplateBuilderAvatarData";
import {
	ConfigurationField,
	ConfigurationFieldContainer,
	type ConfigurationFieldDefinition,
} from "./ConfigurationField";

type TemplateConfigurationProps = PropsWithChildren<{
	name: string;
	description: string;
	iconUrl?: string;
	detailsUrl?: string;
	fields?: ConfigurationFieldDefinition[];
}>;

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
				<TemplateBuilderAvatarData
					name={name}
					description={description}
					iconUrl={iconUrl}
					detailsUrl={detailsUrl}
				/>
			</header>

			{fields && fields.length > 0 && (
				<ConfigurationFieldContainer>
					{fields.map((field) => (
						<ConfigurationField key={field.id} field={field} />
					))}
				</ConfigurationFieldContainer>
			)}
			{children}
		</section>
	);
};
