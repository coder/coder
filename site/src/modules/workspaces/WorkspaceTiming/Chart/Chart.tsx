import { cn } from "cn";
import { ChevronRightIcon } from "lucide-react";
import React, { useEffect, useRef } from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	SearchField,
	type SearchFieldProps,
} from "#/components/SearchField/SearchField";
import type { BarColors } from "./Bar";

export const Chart = (props: React.HTMLProps<HTMLDivElement>) => {
	return (
		<div
			{...props}
			className={cn("flex h-full flex-col", props.className)}
			style={{
				"--header-height": "40px",
				"--section-padding": "16px",
				"--x-axis-rows-gap": "20px",
				"--y-axis-width": "200px",
				...props.style,
			}}
		/>
	);
};

export const ChartContent: React.FC<React.HTMLProps<HTMLDivElement>> = ({
	className,
	children,
	...props
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const viewportRef = useRef<HTMLDivElement>(null);

	// Display a scroll mask when the content is scrollable. Remove the mask when
	// the scroll reaches the bottom to ensure the last item is visible.
	useEffect(() => {
		const containerEl = containerRef.current;
		const viewportEl = viewportRef.current;
		if (!containerEl || !viewportEl) return;

		const hasScroll = viewportEl.scrollHeight > viewportEl.clientHeight;
		containerEl.style.setProperty(
			"--scroll-mask-opacity",
			hasScroll ? "1" : "0",
		);

		const handler = () => {
			if (!hasScroll) {
				return;
			}
			const isBottom =
				viewportEl.scrollTop + viewportEl.clientHeight >=
				viewportEl.scrollHeight;
			containerEl.style.setProperty(
				"--scroll-mask-opacity",
				isBottom ? "0" : "1",
			);
		};
		viewportEl.addEventListener("scroll", handler);
		return () => viewportEl.removeEventListener("scroll", handler);
	}, []);

	return (
		<div
			{...props}
			ref={containerRef}
			className={cn(
				"relative flex min-h-0 flex-1 text-xs font-medium",
				className,
			)}
		>
			<ScrollArea
				className="min-h-0 flex-1"
				viewportRef={viewportRef}
				orientation="both"
			>
				<div className="flex items-stretch">{children}</div>
			</ScrollArea>
			<div
				aria-hidden="true"
				className="pointer-events-none absolute inset-x-0 bottom-0 z-1 h-[100px] transition-opacity duration-200 opacity-(--scroll-mask-opacity) [background:linear-gradient(180deg,rgba(0,0,0,0)_0%,var(--surface-primary)_81.93%)]"
			/>
		</div>
	);
};

export const ChartToolbar = (props: React.HTMLProps<HTMLDivElement>) => {
	return (
		<div
			{...props}
			className={cn(
				"flex items-stretch border-b border-border text-xs",
				"border-solid border-0 border-b",
				props.className,
			)}
		/>
	);
};

type ChartBreadcrumb = {
	label: string;
	onClick?: () => void;
};

type ChartBreadcrumbsProps = {
	breadcrumbs: ChartBreadcrumb[];
};

export const ChartBreadcrumbs: React.FC<ChartBreadcrumbsProps> = ({
	breadcrumbs,
}) => {
	return (
		<ul className="m-0 flex w-(--y-axis-width) shrink-0 list-none items-center gap-1 p-(--section-padding) leading-none">
			{breadcrumbs.map((b, i) => {
				const isLast = i === breadcrumbs.length - 1;
				return (
					<React.Fragment key={b.label}>
						<li className={cn(i === 0 && "text-content-secondary")}>
							{isLast ? (
								b.label
							) : (
								<button
									type="button"
									className="cursor-pointer border-0 bg-transparent p-0 text-inherit hover:text-content-primary"
									onClick={b.onClick}
								>
									{b.label}
								</button>
							)}
						</li>
						{!isLast && (
							<li role="presentation" className="text-content-secondary">
								<ChevronRightIcon className="size-[14px]" />
							</li>
						)}
					</React.Fragment>
				);
			})}
		</ul>
	);
};

export const ChartSearch = (props: SearchFieldProps) => {
	return (
		<SearchField
			className="flex-1 h-12 rounded-none border-y-0 border-r-0 mr-4"
			{...props}
		/>
	);
};

export type ChartLegend = {
	label: string;
	colors?: BarColors;
};

type ChartLegendsProps = {
	legends: ChartLegend[];
};

export const ChartLegends: React.FC<ChartLegendsProps> = ({ legends }) => {
	return (
		<ul className="m-0 flex list-none items-center gap-6 pr-(--section-padding) p-0">
			{legends.map((l) => (
				<li
					key={l.label}
					className="flex items-center gap-2 font-medium leading-none"
				>
					<div
						className="size-[18px] rounded border border-solid bg-surface-primary"
						style={{
							borderColor: l.colors?.stroke,
							backgroundColor: l.colors?.fill,
						}}
					/>
					{l.label}
				</li>
			))}
		</ul>
	);
};
