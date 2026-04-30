import { WrenchIcon } from "lucide-react";
import type { FC, HTMLAttributes, PropsWithChildren } from "react";
import { DisabledBadge, EnabledBadge } from "#/components/Badges/Badges";
import { cn } from "#/utils/cn";

export const OptionName: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className="block text-sm font-medium text-content-primary">
			{children}
		</span>
	);
};

export const OptionDescription: FC<PropsWithChildren> = ({ children }) => {
	return <span className="text-sm font-normal">{children}</span>;
};

interface OptionValueProps {
	children?: boolean | number | string | string[] | Record<string, boolean>;
}

export const OptionValue: FC<OptionValueProps> = (props) => {
	const { children: value } = props;
	const optionClassName =
		"text-sm font-mono [overflow-wrap:anywhere] select-all [&_ul]:p-4";

	if (typeof value === "boolean") {
		return (
			<div className="option-value-boolean">
				{value ? <EnabledBadge /> : <DisabledBadge />}
			</div>
		);
	}

	if (typeof value === "number") {
		return (
			<span className={cn("option-value-number", optionClassName)}>
				{value}
			</span>
		);
	}

	if (!value || value.length === 0) {
		return (
			<span className={cn("option-value-empty", optionClassName)}>Not set</span>
		);
	}

	if (typeof value === "string") {
		return (
			<span className={cn("option-value-string", optionClassName)}>
				{value}
			</span>
		);
	}

	if (typeof value === "object" && !Array.isArray(value)) {
		return (
			<ul className="option-array list-none">
				{Object.entries(value)
					.sort((a, b) => a[0].localeCompare(b[0]))
					.map(([option, isEnabled]) => (
						<li
							key={option}
							className={cn(
								`option-array-item-${option}`,
								isEnabled
									? "option-enabled"
									: "option-disabled ml-8 text-content-disabled",
								optionClassName,
							)}
						>
							<div className="inline-flex items-center">
								{isEnabled && <WrenchIcon className="size-4 mx-2" />}
								{option}
							</div>
						</li>
					))}
			</ul>
		);
	}

	if (Array.isArray(value)) {
		return (
			<ul className="option-array list-inside">
				{value.map((item) => (
					<li key={item} className={optionClassName}>
						{item}
					</li>
				))}
			</ul>
		);
	}

	return (
		<span className={cn("option-value-json", optionClassName)}>
			{JSON.stringify(value)}
		</span>
	);
};

type OptionConfigProps = HTMLAttributes<HTMLDivElement> & { isSource: boolean };

// OptionConfig takes a isSource bool to indicate if the Option is the source of the configured value.
export const OptionConfig: FC<OptionConfigProps> = ({
	isSource,
	className,
	...attrs
}) => {
	return (
		<div
			{...attrs}
			className={cn(
				"inline-flex items-center gap-1.5 rounded border border-solid p-1.5",
				"font-mono text-[13px] font-semibold leading-none",
				"border-border-secondary bg-surface-secondary",
				isSource &&
					"border-content-link [&_[data-slot=option-config-flag]]:bg-content-link",
				className,
			)}
		/>
	);
};

export const OptionConfigFlag: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<div
			{...props}
			data-slot="option-config-flag"
			className={cn(
				"block rounded-[1px] bg-border-secondary px-1 py-0.5",
				"text-[10px] font-semibold leading-none",
				props.className,
			)}
		/>
	);
};
