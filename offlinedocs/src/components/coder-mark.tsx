type Props = {
	className?: string;
};

// Coder brand mark (coder.com/brand). Uses `currentColor` so it inherits the
// surrounding text color and adapts to light/dark automatically.
export function CoderMark({ className }: Props) {
	return (
		<svg
			xmlns="http://www.w3.org/2000/svg"
			viewBox="0 0 40 40"
			fill="currentColor"
			className={className}
			aria-hidden="true"
		>
			<path d="m12.7852 12c5.6651 0 8.8417 2.6816 8.9492 6.6289l-4.8926.1504c-.1288-2.1882-2.0717-3.6259-4.0566-3.583-2.7252.0538-4.74222 1.8674-4.74222 4.7422.00022 2.8744 2.01712 4.6551 4.74222 4.6553 1.9849 0 3.884-1.3733 4.0986-3.5616l4.8936.1074c-.1289 4.0119-3.4987 6.7364-8.9922 6.7364-5.49331-.0002-9.78492-3.1107-9.78518-7.9375 0-4.8486 4.12004-7.9384 9.78518-7.9385zm24.2138.4688v15.0185h-12.875v-15.0185z" />
		</svg>
	);
}
