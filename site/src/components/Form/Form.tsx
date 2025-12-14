import { AlphaBadge, DeprecatedBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";
import {
	type ComponentProps,
	createContext,
	type FC,
	forwardRef,
	type HTMLProps,
	type ReactNode,
	useContext,
} from "react";
import { cn } from "utils/cn";

type FormContextValue = { direction?: "horizontal" | "vertical" };

const FormContext = createContext<FormContextValue>({
	direction: "horizontal",
});

type FormProps = HTMLProps<HTMLFormElement> & {
	direction?: FormContextValue["direction"];
};

export const Form: FC<FormProps> = ({ direction, ...formProps }) => {
	return (
		<FormContext.Provider value={{ direction }}>
			<form
				{...formProps}
				className={cn(
					"flex flex-col gap-16",
					direction === "horizontal" && "md:gap-20",
					direction !== "horizontal" && "md:gap-10",
				)}
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
}

export const FormSection = forwardRef<HTMLDivElement, FormSectionProps>(
	(
		{
			children,
			title,
			description,
			classes = {},
			alpha = false,
			deprecated = false,
		},
		ref,
	) => {
		const { direction } = useContext(FormContext);

		return (
			<section
				ref={ref}
				className={cn(
					classes.root,
					"flex flex-col items-start gap-4 lg:gap-6",
					direction === "horizontal" && "flex-col lg:flex-row lg:gap-[120px]",
				)}
			>
				<div
					className={cn(
						classes.sectionInfo,
						"w-full position-[initial] top-6 flex-shrink-0",
						direction === "horizontal" && "max-w-[312px] lg:sticky",
					)}
				>
					<h2
						className={cn(
							classes.infoTitle,
							"text-xl text-content-primary font-medium m-0 mb-2",
							"flex flex-row items-center gap-3",
						)}
					>
						{title}
						{alpha && <AlphaBadge />}
						{deprecated && <DeprecatedBadge />}
					</h2>
					<div className={"text-content-secondary text-sm leading-relaxed m-0"}>
						{description}
					</div>
				</div>

				{children}
			</section>
		);
	},
);

export const FormFields: FC<ComponentProps<typeof Stack>> = (props) => {
	return (
		<Stack direction="column" spacing={3} {...props} className={"w-full"} />
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
