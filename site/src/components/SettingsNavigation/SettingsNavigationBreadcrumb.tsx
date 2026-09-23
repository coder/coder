import { cn } from "cn";
import { CheckIcon } from "lucide-react";
import { motion } from "motion/react";
import type { FC, ReactNode } from "react";
import { Link } from "react-router";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbList,
	BreadcrumbPage,
} from "#/components/Breadcrumb/Breadcrumb";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { NavigationTitle, NavigationToggle } from "./SettingsNavigationSidebar";
import type {
	LocatedSettingsNavigationItem,
	SettingsNavigationItem,
	SettingsNavigationSection,
} from "./types";
import { shouldUseOptimisticSelection } from "./types";

type PageCrumbProps = {
	sections: readonly SettingsNavigationSection[];
	activeItemId: string | undefined;
	label: string;
	onSelect: (item: SettingsNavigationItem) => void;
};

const PageCrumb: FC<PageCrumbProps> = ({
	sections,
	activeItemId,
	label,
	onSelect,
}) => {
	const populatedSections = sections.filter(
		(section) => section.items.length > 0,
	);
	const pageCount = populatedSections.reduce(
		(total, section) => total + section.items.length,
		0,
	);

	if (pageCount <= 1) {
		return (
			<BreadcrumbPage className="truncate px-1 text-content-primary">
				{label}
			</BreadcrumbPage>
		);
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="subtle"
					size="sm"
					className="group min-w-0 gap-1.5 px-1 text-sm font-medium text-content-primary"
					aria-current="page"
				>
					<span className="truncate">{label}</span>
					<ChevronDownIcon className="size-icon-sm! shrink-0" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				size="sm"
				className="max-h-[70vh] overflow-y-auto"
			>
				{populatedSections.map((section, index) => (
					<div key={section.id}>
						{index > 0 && <DropdownMenuSeparator />}
						{section.label && (
							<DropdownMenuLabel className="opacity-70">
								{section.label}
							</DropdownMenuLabel>
						)}
						{section.items.map((item) => (
							<DropdownMenuItem key={item.id} asChild>
								<Link
									to={item.href}
									onClick={(event) => {
										if (shouldUseOptimisticSelection(event)) {
											onSelect(item);
										}
									}}
									className={cn(
										"justify-between gap-4",
										section.label && "pl-4",
										item.id === activeItemId && "text-content-primary",
									)}
								>
									{item.label}
									{item.id === activeItemId && <CheckIcon />}
								</Link>
							</DropdownMenuItem>
						))}
					</div>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

type SettingsNavigationBreadcrumbProps = {
	title: string;
	titleLayoutId: string;
	dividerLayoutId: string;
	sections: readonly SettingsNavigationSection[];
	active: LocatedSettingsNavigationItem | undefined;
	pageAdornment?: ReactNode;
	onExpand: () => void;
	onSelect: (item: SettingsNavigationItem) => void;
	animateControls: boolean;
	toggleRef: React.Ref<HTMLButtonElement>;
};

export const SettingsNavigationBreadcrumb: FC<
	SettingsNavigationBreadcrumbProps
> = ({
	title,
	titleLayoutId,
	dividerLayoutId,
	sections,
	active,
	pageAdornment,
	onExpand,
	onSelect,
	animateControls,
	toggleRef,
}) => (
	<div className="flex flex-col">
		<Breadcrumb className="flex h-14 items-center gap-1 px-3">
			<NavigationToggle
				collapsed
				onClick={onExpand}
				animateIn={animateControls}
				toggleRef={toggleRef}
			/>
			<BreadcrumbList className="my-0 flex-nowrap gap-1 pl-1 sm:gap-1">
				<BreadcrumbItem className="min-w-0">
					<NavigationTitle title={title} layoutId={titleLayoutId} collapsed />
				</BreadcrumbItem>
				{active && (
					<motion.li
						className="flex min-w-0 items-center gap-2 text-content-secondary"
						initial={animateControls ? { opacity: 0, x: -16 } : false}
						animate={{ opacity: 1, x: 0 }}
						transition={{
							type: "spring",
							stiffness: 380,
							damping: 28,
							delay: 0.18,
						}}
					>
						<span aria-hidden className="text-content-disabled">
							/
						</span>
						{pageAdornment}
						<PageCrumb
							sections={sections}
							activeItemId={active.item.id}
							label={active.item.label}
							onSelect={onSelect}
						/>
					</motion.li>
				)}
			</BreadcrumbList>
		</Breadcrumb>
		<motion.div
			layoutId={dividerLayoutId}
			className="h-px bg-border"
			transition={{ layout: { duration: 0.12, ease: "easeOut" } }}
		/>
	</div>
);
