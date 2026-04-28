import type { FC, ReactNode } from "react";
import {
	FeatureStageBadge,
	type featureStageBadgeTypes,
} from "#/components/FeatureStageBadge/FeatureStageBadge";

type SectionLayout = "fixed" | "fluid";

interface SectionProps {
	// Useful for testing
	id?: string;
	title?: ReactNode | string;
	description?: ReactNode;
	toolbar?: ReactNode;
	alert?: ReactNode;
	layout?: SectionLayout;
	className?: string;
	children?: ReactNode;
	featureStage?: keyof typeof featureStageBadgeTypes;
}

const DESCRIPTION_CLASS =
	"text-content-secondary text-base m-0 mt-1 leading-normal";

export const Section: FC<SectionProps> = ({
	id,
	title,
	description,
	toolbar,
	alert,
	className = "",
	children,
	layout = "fixed",
	featureStage,
}) => {
	return (
		<section className={className} id={id} data-testid={id}>
			<div className={layout === "fluid" ? "max-w-full" : "max-w-[500px]"}>
				{(title || description) && (
					<div className="mb-6 flex flex-row justify-between">
						<div>
							{title && (
								<div className="flex flex-row items-center gap-4">
									<h4 className="text-2xl font-medium m-0 mb-2">{title}</h4>
									{featureStage && (
										<FeatureStageBadge
											contentType={featureStage}
											size="md"
											className="mb-[5px]"
										/>
									)}
								</div>
							)}
							{description && typeof description === "string" && (
								<p className={DESCRIPTION_CLASS}>{description}</p>
							)}
							{description && typeof description !== "string" && (
								<div className={DESCRIPTION_CLASS}>{description}</div>
							)}
						</div>
						{toolbar && <div>{toolbar}</div>}
					</div>
				)}
				{alert && <div className="mb-2">{alert}</div>}
				{children}
			</div>
		</section>
	);
};
