import { cn } from "cn";

export const YAxis: React.FC<React.HTMLProps<HTMLDivElement>> = (props) => {
	return (
		<div
			{...props}
			className={cn("shrink-0", props.className)}
			style={{
				...props.style,
				width: "var(--y-axis-width)",
			}}
		/>
	);
};

export const YAxisSection: React.FC<React.HTMLProps<HTMLDivElement>> = (
	props,
) => {
	return (
		<section
			{...props}
			className={cn(
				"not-first-of-type:border-solid",
				"not-first-of-type:border-0",
				"not-first-of-type:border-t",
			)}
		/>
	);
};

export const YAxisHeader: React.FC<React.HTMLProps<HTMLSpanElement>> = (
	props,
) => {
	return (
		<header
			{...props}
			className={cn(
				"flex items-center",
				"sticky top-0 bg-surface-primary",
				"text-xs font-medium text-content-secondary",
				"border-solid border-0 border-b",
			)}
			style={{
				height: "var(--header-height)",
				paddingLeft: "var(--section-padding)",
				paddingRight: "var(--section-padding)",
			}}
		/>
	);
};

export const YAxisLabels: React.FC<React.HTMLProps<HTMLUListElement>> = (
	props,
) => {
	return (
		<ul
			{...props}
			className={cn(
				"m-0 list-none text-right",
				"flex flex-col",
				props.className,
			)}
			style={{
				...props.style,
				gap: "var(--x-axis-rows-gap)",
				padding: "var(--section-padding)",
			}}
		/>
	);
};

type YAxisLabelProps = Omit<React.HTMLProps<HTMLLIElement>, "id"> & {
	id: string;
};

export const YAxisLabel: React.FC<YAxisLabelProps> = ({ id, ...props }) => {
	return (
		<li
			{...props}
			className={cn(
				"flex items-center",
				"[&>*]:block [&>*]:w-full [&>*]:overflow-hidden [&>*]:text-ellipsis [&>*]:whitespace-nowrap",
				props.className,
			)}
			id={encodeURIComponent(id)}
		>
			<span>{props.children}</span>
		</li>
	);
};
