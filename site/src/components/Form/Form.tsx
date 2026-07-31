import type { ComponentProps, FC, HTMLProps, ReactNode, Ref } from "react";
import { AlphaBadge, DeprecatedBadge } from "#/components/Badges/Badges";
import { cn } from "#/utils/cn";

export const Form: FC<HTMLProps<HTMLFormElement>> = ({
	className,
	...formProps
}) => {
	return (
		<form
			{...formProps}
			className={cn("flex flex-col gap-16 md:gap-10", className)}
		/>
	);
};

interface FormSectionProps {
	children?: ReactNode;
	title: ReactNode;
	description: ReactNode;
	classes?: {
		root?: string;
		sectionInfo?: string;
		infoTitle?: string;
	};
	alpha?: boolean;
	deprecated?: boolean;
	ref?: Ref<HTMLElement>;
}

export const FormSection: FC<FormSectionProps> = ({
	children,
	title,
	description,
	classes = {},
	alpha = false,
	deprecated = false,
	ref,
}) => {
	return (
		<section
			ref={ref}
			className={cn("flex items-start flex-col gap-4 lg:gap-6", classes.root)}
		>
			<div className={cn("w-full shrink-0 top-24", classes.sectionInfo)}>
				<header className="flex items-center gap-4">
					<h2
						className={cn(
							"m-0 mb-2 flex flex-row items-center gap-3 text-xl font-medium text-content-primary",
							classes.infoTitle,
						)}
					>
						{title}
					</h2>
					{alpha && <AlphaBadge />}
					{deprecated && <DeprecatedBadge />}
				</header>
				<div className="m-0 text-sm leading-[160%] text-content-secondary">
					{description}
				</div>
			</div>

			{children}
		</section>
	);
};

export const FormFields: FC<ComponentProps<"div">> = ({
	className,
	...props
}) => {
	return (
		<div className={cn("flex w-full flex-col gap-6", className)} {...props} />
	);
};

export const FormFooter: FC<HTMLProps<HTMLDivElement>> = ({
	className,
	...props
}) => (
	<footer
		className={cn("flex items-center justify-end space-x-2 mt-2", className)}
		{...props}
	/>
);
