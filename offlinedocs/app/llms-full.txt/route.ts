import { llms } from "@/lib/llms";

// `/llms-full.txt`: every page's Markdown in one file.
export const { GET } = llms.full;
export const dynamic = "force-static";
