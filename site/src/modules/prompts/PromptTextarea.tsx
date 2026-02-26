import type { FC, Ref } from "react";
import TextareaAutosize, {
	type TextareaAutosizeProps,
} from "react-textarea-autosize";

type PromptTextareaProps = TextareaAutosizeProps & {
	isSubmitting?: boolean;
	ref?: Ref<HTMLTextAreaElement>;
};

export const PromptTextarea: FC<PromptTextareaProps> = ({
	isSubmitting,
	className,
	ref,
	...props
}) => {
	return (
		<div className="relative">
			<TextareaAutosize
				ref={ref}
				{...props}
				className={`border-0 px-3 py-2 resize-none w-full h-full bg-transparent rounded-lg
					outline-none flex min-h-24 text-sm shadow-sm text-content-primary
					placeholder:text-content-secondary md:text-sm ${props.readOnly || props.disabled || isSubmitting ? "opacity-60 cursor-not-allowed" : ""} ${className ?? ""}`}
			/>
			{isSubmitting && (
				<div className="absolute inset-0 pointer-events-none overflow-hidden">
					<div
						className={`absolute top-0 w-0.5 h-full
						bg-green-400/90 animate-caret-scan rounded-sm
						shadow-[-15px_0_15px_rgba(0,255,0,0.9),-30px_0_30px_rgba(0,255,0,0.7),-45px_0_45px_rgba(0,255,0,0.5),-60px_0_60px_rgba(0,255,0,0.3)]`}
					/>
				</div>
			)}
		</div>
	);
};
