import { llms } from "@/lib/llms";

// `/llms.txt`: every page in the export, with its description.
export const { GET } = llms.index;
// Statically analyzable segment config (Turbopack rejects re-exported
// bindings).
export const dynamic = "force-static";
