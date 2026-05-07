import type { FC, HTMLAttributes, ReactNode } from "react";
export const HorizontalContainer: FC<HTMLAttributes<HTMLDivElement>> = ({
	...attrs
}) => {
	return <div className="flex flex-col gap-16 md:gap-20" {...attrs} />;
};

interface HorizontalSectionProps
	extends Omit<HTMLAttributes<HTMLElement>, "title"> {
	title: ReactNode;
	description: ReactNode;
	children?: ReactNode;
}

export const HorizontalSection: FC<HorizontalSectionProps> = ({
	children,
	title,
	description,
	...attrs
}) => {
	return (
		<section className="flex flex-col gap-4 lg:flex-row lg:gap-32" {...attrs}>
			<div className="w-full shrink-0 lg:sticky lg:top-6 lg:max-w-[312px]">
				<h2 className="m-0 mb-2 flex flex-row items-center gap-3 text-xl font-normal text-content-primary">
					{title}
				</h2>
				<div className="m-0 text-sm leading-[160%] text-content-secondary">
					{description}
				</div>
			</div>

			{children}
		</section>
	);
};
