type TerminalPreviewProps = {
	fontFamily: string;
	fontSize: number;
};

const Prompt: React.FC = () => (
	<>
		<span className="text-[#8ae234]">coder@workspace</span>:
		<span className="text-[#729fcf]">~/project</span>${" "}
	</>
);

/**
 * Static mock of the web terminal, using xterm's default colors, so users can
 * compare terminal fonts and sizes without opening a workspace.
 */
export const TerminalPreview: React.FC<TerminalPreviewProps> = ({
	fontFamily,
	fontSize,
}) => {
	return (
		<div
			aria-hidden
			className="w-full max-w-lg overflow-hidden rounded-md border border-solid border-border bg-black p-2 text-white select-none"
			style={{ fontFamily, fontSize }}
		>
			<pre className="m-0 whitespace-pre-wrap break-all font-[inherit]">
				<Prompt />
				ls
				{"\n"}
				<span className="text-[#729fcf]">site</span>
				{"  README.md  go.mod  main.go\n"}
				<Prompt />
				{'echo "0O 1lI {} => != ->"\n'}
				{"0O 1lI {} => != ->\n"}
				<Prompt />
				<span className="bg-white"> </span>
			</pre>
		</div>
	);
};
