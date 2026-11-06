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

// Ordinary Go that exercises common ligatures (:= <- != && >= //) and
// easily confused glyphs (0 and O, 1 and l).
const sampleFile = [
	"package main",
	"",
	'import "fmt"',
	"",
	"func main() {",
	"\tready := make(chan int, 1)",
	"\tgo func() { ready <- 10 }()",
	"\tif n := <-ready; n != 0 && n >= 1 {",
	'\t\tfmt.Println("Online:", n) // ok',
	"\t}",
	"}",
	"",
].join("\n");

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
			className="w-full min-w-0 flex-1 overflow-hidden rounded-md border border-solid border-border bg-black p-2 text-white select-none"
			style={{ fontFamily, fontSize }}
		>
			<pre className="m-0 whitespace-pre-wrap break-all font-[inherit]">
				<Prompt />
				ls
				{"\n"}
				<span className="text-[#729fcf]">site</span>
				{"  README.md  go.mod  main.go\n"}
				<Prompt />
				{"cat main.go\n"}
				{sampleFile}
				<Prompt />
				<span className="bg-white"> </span>
			</pre>
		</div>
	);
};
