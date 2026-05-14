import { useTheme } from "@emotion/react";
import {
	type ComponentProps,
	createContext,
	type FC,
	type HTMLProps,
	type ReactNode,
	useContext,
} from "react";
import { AlphaBadge, DeprecatedBadge } from "#/components/Badges/Badges";
import { cn } from "#/utils/cn";

type FormContextValue = { direction?: "horizontal" | "vertical" };

const FormContext = createContext<FormContextValue>({
	direction: "horizontal",
});

type FormProps = HTMLProps<HTMLFormElement> & {
	direction?: FormContextValue["direction"];
};

export const Form: FC<FormProps> = ({ direction, ...formProps }) => {
	const theme = useTheme();

	return (
		<FormContext.Provider value={{ direction }}>
			<form
				{...formProps}
				css={{
					display: "flex",
					flexDirection: "column",
					gap: direction === "horizontal" ? 80 : 40,

					[theme.breakpoints.down("md")]: {
						gap: 64,
					},
				}}
			/>
		</FormContext.Provider>
	);
};

export const HorizontalForm: FC<HTMLProps<HTMLFormElement>> = ({
	children,
	...formProps
}) => {
	return (
		<Form direction="horizontal" {...formProps}>
			{children}
		</Form>
	);
};

export const VerticalForm: FC<HTMLProps<HTMLFormElement>> = ({
	children,
	...formProps
}) => {
	return (
		<Form direction="vertical" {...formProps}>
			{children}
		</Form>
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
	ref?: React.Ref<HTMLElement>;
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
	const { direction } = useContext(FormContext);

	return (
		<section
			ref={ref}
			className={cn(
				"flex items-start flex-col gap-4 lg:gap-6",
				direction === "horizontal" && "lg:flex-row lg:gap-[120px]",
				classes.root,
			)}
		>
			<div
				className={cn(
					"w-full shrink-0 top-6",
					direction === "horizontal" && "lg:sticky lg:max-w-[312px]",
					classes.sectionInfo,
				)}
			>
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
