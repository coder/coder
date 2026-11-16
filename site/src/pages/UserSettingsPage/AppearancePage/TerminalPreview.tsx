import { Label } from "#/components/Label/Label";
import { useAuthenticated } from "#/hooks/useAuthenticated";

type TerminalPreviewProps = {
	fontFamily: string;
	fontSize: number;
};

const Prompt: React.FC = () => {
	const { user: me } = useAuthenticated();
	return (
		<>
			<span className="text-[#8ae234]">{me.username}@wibble</span>:
			<span className="text-[#729fcf]">wobble</span>${" "}
		</>
	);
};

// Taken from the "Concurrent Prime Sieve" example on https://go.dev/play/
const sampleFile = `package main

import "fmt"

// Send the sequence 2, 3, 4, ... to channel 'ch'.
func Generate(ch chan<- int) {
	for i := 2; ; i++ {
		ch <- i // Send 'i' to channel 'ch'.
	}
}

// Copy the values from channel 'in' to channel 'out',
// removing those divisible by 'prime'.
func Filter(in <-chan int, out chan<- int, prime int) {
	for {
		i := <-in // Receive value from 'in'.
		if i%prime != 0 {
			out <- i // Send 'i' to 'out'.
		}
	}
}

// The prime sieve: Daisy-chain Filter processes.
func main() {
	ch := make(chan int) // Create a new channel.
	go Generate(ch)      // Launch Generate goroutine.
	for i := 0; i < 10; i++ {
		prime := <-ch
		fmt.Println(prime)
		ch1 := make(chan int)
		go Filter(ch, ch1, prime)
		ch = ch1
	}
}
`;

/**
 * Static mock of the web terminal, using xterm's default colors, so users can
 * compare terminal fonts and sizes without opening a workspace.
 */
export const TerminalPreview: React.FC<TerminalPreviewProps> = ({
	fontFamily,
	fontSize,
}) => {
	return (
		<div className="flex flex-col gap-2">
			<Label className="text-sm font-medium">Preview</Label>
			<div
				aria-hidden
				className="w-full max-h-72 min-w-0 flex-1 overflow-y-auto rounded-md border border-solid border-border bg-black p-2 text-white select-none"
				style={{ fontFamily, fontSize }}
			>
				<pre className="m-0 whitespace-pre-wrap break-all font-[inherit]">
					<Prompt />
					ls
					{"\n"}
					{"README.md  go.mod  main.go\n"}
					<Prompt />
					{"cat main.go\n"}
					{sampleFile}
					<Prompt />
					<span className="bg-white"> </span>
				</pre>
			</div>
		</div>
	);
};
