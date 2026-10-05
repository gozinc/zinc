import type { APIRoute } from "astro";
import { sections, summary, url } from "../lib/llms";

export const GET: APIRoute = async () => {
  const parts = [summary, "", "The whole documentation as one file: https://zinc.carbonsoft.sh/llms-full.txt", ""];
  for (const section of await sections()) {
    parts.push(`## ${section.title}`, "");
    for (const page of section.pages) parts.push(`- [${page.data.title}](${url(page)}): ${page.data.description}`);
    parts.push("");
  }
  return new Response(parts.join("\n"), { headers: { "Content-Type": "text/plain; charset=utf-8" } });
};
