import { Input } from "components/Input/Input";
import { SearchIcon } from "lucide-react";
import type { FC, HTMLProps } from "react";
import { cn } from "utils/cn";

export const SearchInput: FC<HTMLProps<HTMLInputElement>> = ({
	className,
	...props
}) => {
	return (
		<div className="relative w-[400px]">
			<Input
				{...props}
				className={cn([
					"pl-[34px] pr-2 h-[30px] text-xs md:text-xs rounded-md text-content-primary",
					className,
				])}
			/>
			<SearchIcon className="left-2 top-1.5 absolute size-icon-sm text-content-secondary p-0.5" />
		</div>
	);
};
